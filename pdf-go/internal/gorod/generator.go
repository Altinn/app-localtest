package gorod

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"

	"altinn/pdf/internal/types"
)

type Rod struct {
	workers        []*browserWorker
	queue          chan workerRequest
	wg             sync.WaitGroup
	browserVersion types.BrowserVersion
}

func New() (*Rod, error) {
	workerCount := types.MaxConcurrency
	fmt.Printf("Starting Rod with %d browser workers\n", workerCount)

	generator := &Rod{
		workers: make([]*browserWorker, workerCount),
		queue:   make(chan workerRequest, workerCount*2),
	}

	// We only need the queue to be initialized before returning the generator to main
	// We can initialize the workers asynchronously, and they will start
	// consuming requests from the queue.
	// We should be reasonably sure that the workers are quick to start (<1s, otherwise we might get issues)
	go func() {
		fmt.Printf("Initializing Rod\n")

		// Create launcher with same browser options as ChromeDP
		launcher := createBrowserLauncher()

		logArgs(launcher)

		// Launch browser and get version info using a temporary browser instance
		url, err := launcher.Launch()
		if err != nil {
			log.Fatalf("Failed to launch browser: %v", err)
		}

		browser := rod.New().ControlURL(url)
		err = browser.Connect()
		if err != nil {
			log.Fatalf("Failed to connect to browser: %v", err)
		}
		defer func() {
			if err := browser.Close(); err != nil {
				log.Printf("Failed to close temporary browser: %v", err)
			}
			launcher.Kill()
		}()

		version, err := browser.Version()
		if err != nil {
			log.Fatalf("Failed to get browser version: %v", err)
		}

		generator.browserVersion = types.BrowserVersion{
			Product:         version.Product,
			ProtocolVersion: version.ProtocolVersion,
			Revision:        version.Revision,
			UserAgent:       version.UserAgent,
			JSVersion:       version.JsVersion,
		}

		fmt.Printf("Chrome version: %s (revision: %s, protocol: %s)\n", version.Product, version.Revision, version.ProtocolVersion)

		for i := range workerCount {
			generator.wg.Add(1)
			go func(i int) {
				defer generator.wg.Done()
				fmt.Printf("Starting browser worker %d\n", i)
				worker := newBrowserWorker(i)

				generator.workers[i] = worker
				fmt.Printf("Browser worker %d started successfully\n", i)
				worker.run(generator.queue)
				fmt.Printf("Browser worker %d terminated\n", i)
			}(i)
		}
	}()

	return generator, nil
}

func (g *Rod) Generate(ctx context.Context, request types.PdfRequest) (*types.PdfResult, *types.PDFError) {
	responder := make(chan workerResponse, 1)
	req := workerRequest{
		request:   request,
		responder: responder,
		ctx:       ctx,
		cleanedUp: false,
	}

	select {
	case g.queue <- req:
		break
	case <-ctx.Done():
		return nil, types.NewPDFError(types.ErrClientDropped, "", ctx.Err())
	case <-time.After(5 * time.Second):
		fmt.Printf("Request queue full, rejecting request for URL: %s\n", request.URL)
		return nil, types.NewPDFError(types.ErrQueueFull, "", nil)
	}

	select {
	case response := <-responder:
		if response.Error != nil {
			return nil, response.Error
		}
		return &types.PdfResult{
			Data:    response.Data,
			Browser: g.browserVersion,
		}, nil
	case <-ctx.Done():
		return nil, types.NewPDFError(types.ErrClientDropped, "", ctx.Err())
	case <-time.After(30 * time.Second):
		fmt.Printf("Client timeout waiting for PDF generation (30s) for URL: %s - abandoning request\n", request.URL)
		return nil, types.NewPDFError(types.ErrTimeout, "", nil)
	}
}

func (g *Rod) Close() error {
	close(g.queue)
	g.wg.Wait()
	return nil
}

type workerRequest struct {
	request   types.PdfRequest
	responder chan workerResponse
	ctx       context.Context
	cleanedUp bool
}

func (r *workerRequest) tryRespondOk(data []byte) {
	if r.responder != nil {
		response := workerResponse{
			Data:  data,
			Error: nil,
		}
		select {
		case r.responder <- response:
			break
		default:
			fmt.Printf("Worker: client abandoned request (likely timed out), dropping response for URL: %s\n", r.request.URL)
		}
		r.responder = nil
	}
}

func (r *workerRequest) tryRespondError(err *types.PDFError) {
	if r.responder != nil {
		response := workerResponse{
			Data:  nil,
			Error: err,
		}
		select {
		case r.responder <- response:
			break
		default:
			fmt.Printf("Worker: client abandoned request (likely timed out), dropping response for URL: %s\n", r.request.URL)
		}
		r.responder = nil
	}
}

func (r *workerRequest) hasResponded() bool {
	return r.responder == nil
}

type workerResponse struct {
	Data  []byte
	Error *types.PDFError
}

type browserWorker struct {
	id         int
	browser    *rod.Browser
	page       *rod.Page
	currentUrl string
	launcher   *launcher.Launcher
}

func newBrowserWorker(id int) *browserWorker {
	// Create a new launcher instance for this worker with the same configuration
	// Each worker needs its own browser process
	workerLauncher := createBrowserLauncher()
	// Override user data directory for this worker
	workerLauncher = workerLauncher.UserDataDir(fmt.Sprintf("/tmp/browser-%d", id))
	url, err := workerLauncher.Launch()
	if err != nil {
		log.Fatalf("Worker %d failed to launch browser: %v", id, err)
	}

	browser := rod.New().ControlURL(url)
	err = browser.Connect()
	if err != nil {
		log.Fatalf("Worker %d failed to connect to browser: %v", id, err)
	}

	page, err := browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		browser.Close()
		log.Fatalf("Worker %d failed to create page: %v", id, err)
	}

	w := &browserWorker{
		id:         id,
		browser:    browser,
		page:       page,
		currentUrl: "",
		launcher:   workerLauncher,
	}

	// Set up console event logging similar to ChromeDP implementation
	go page.EachEvent(func(e *proto.RuntimeConsoleAPICalled) {
		if e.Type == proto.RuntimeConsoleAPICalledTypeError {
			errorJson, err := json.MarshalIndent(e, "", "  ")
			if err != nil {
				errorMsg := fmt.Sprintf("%s - %v", e.Type, e.Args)
				fmt.Printf("[%d, %s] console error: %s\n", w.id, w.currentUrl, errorMsg)
			} else {
				errorJsonStr := string(errorJson)
				fmt.Printf("[%d, %s] console error: %s\n", w.id, w.currentUrl, errorJsonStr)
			}
		}
	})()

	// Set up log event listening similar to ChromeDP implementation
	go page.EachEvent(func(e *proto.LogEntryAdded) {
		if e.Entry.Level == "error" {
			errorJson, err := json.MarshalIndent(e.Entry, "", "  ")
			if err != nil {
				errorMsg := fmt.Sprintf("%s - %s", e.Entry.URL, e.Entry.Text)
				fmt.Printf("[%d, %s] console error: %s\n", w.id, w.currentUrl, errorMsg)
			} else {
				errorJsonStr := string(errorJson)
				fmt.Printf("[%d, %s] console error: %s\n", w.id, w.currentUrl, errorJsonStr)
			}
		}
	})()

	fmt.Printf("Browser worker %d initialized successfully\n", id)
	return w
}

func (w *browserWorker) run(queue <-chan workerRequest) {
	defer func() {
		if err := w.page.Close(); err != nil {
			log.Printf("Worker %d failed to close page: %v", w.id, err)
		}
		if err := w.browser.Close(); err != nil {
			log.Printf("Worker %d failed to close browser: %v", w.id, err)
		}
		if w.launcher != nil {
			w.launcher.Kill()
		}
	}()

	for req := range queue {
		w.currentUrl = req.request.URL
		w.handleRequest(&req)
		w.currentUrl = ""
		// It's important that we always respond, otherwise we leave
		// requests hanging and timing out for no reason
		if !req.hasResponded() {
			log.Fatalf("[%d, %s] did not respond to request\n", w.id, w.currentUrl)
		}
	}
	fmt.Printf("Worker %d shutting down\n", w.id)
}

func (w *browserWorker) handleRequest(req *workerRequest) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[%d, %s] recovered from panic: %v\n", w.id, w.currentUrl, r)
			req.tryRespondError(types.NewPDFError(types.ErrGenerationFail, req.request.URL, fmt.Errorf("%v", r)))
		}
	}()

	if req.ctx.Err() != nil {
		req.tryRespondError(types.NewPDFError(types.ErrClientDropped, "", req.ctx.Err()))
		return
	}

	start := time.Now()

	err := w.generatePdf(req)
	// Cleanup is guaranteed to run and retry via defer in generatePdf
	// The defer block will handle all cleanup logic and retry attempts

	duration := time.Since(start)
	fmt.Printf("[%d, %s] completed PDF request in %.2f seconds\n", w.id, w.currentUrl, duration.Seconds())

	if err != nil {
		req.tryRespondError(w.mapRodError(err))
	}
}

func (w *browserWorker) generatePdf(req *workerRequest) error {
	request := req.request
	page := w.page

	// Ensure cleanup always runs, just like ChromeDP's task-based approach
	defer func() {
		// Navigate back to default
		// It's important that this is here, otherwise the browser isn't really going
		// to navigate if the current request is the same URL as the last request
		// (unlikely, but can happen during tests or due to client retries)
		err := page.Navigate("about:blank")
		if err != nil {
			log.Printf("[%d, %s] failed to navigate out of url: %v", w.id, w.currentUrl, err)
		}

		// TODO: which resources don't have to be cleared? Would be nice to cache static assets
		// one way to find out would be to analyze the user-data-dir in the container before/after clearing data
		// and running some experiments with different storage types left out
		w.cleanupBrowser(req)

		// Retry cleanup if it failed, just like ChromeDP
		if !req.cleanedUp {
			log.Printf("[%d, %s] failed to cleanup storage, retrying...", w.id, w.currentUrl)
			for i := range 3 {
				err := w.cleanupBrowser(req)
				if err != nil {
					log.Printf("[%d, %s] failed to cleanup storage during retry %d, retrying...", w.id, w.currentUrl, i+1)
				}
				if req.cleanedUp {
					break
				}
			}

			if !req.cleanedUp {
				log.Fatalf("[%d, %s] failed to cleanup storage, we're in an unsafe state and can't proceed", w.id, w.currentUrl)
			}
		}
	}()

	// Set cookies
	for _, cookie := range request.Cookies {
		if req.ctx.Err() != nil {
			// It's OK to just return here because we haven't done anything with
			// the users input yet.
			return types.NewPDFError(types.ErrClientDropped, "", req.ctx.Err())
		}

		sameSite := proto.NetworkCookieSameSiteLax
		switch cookie.SameSite {
		case "Strict":
			sameSite = proto.NetworkCookieSameSiteStrict
		case "None":
			sameSite = proto.NetworkCookieSameSiteNone
		}

		err := page.SetCookies([]*proto.NetworkCookieParam{
			{
				Name:     cookie.Name,
				Value:    cookie.Value,
				Domain:   cookie.Domain,
				Path:     "/",
				Secure:   false,
				HTTPOnly: false,
				SameSite: sameSite,
			},
		})
		if err != nil {
			req.tryRespondError(types.NewPDFError(types.ErrSetCookieFail, "", err))
			return nil
		}
	}

	// Now we have potentially set cookies, so it is no longer safe
	// to return errors from tasks because then we won't execute cleanup
	if req.hasResponded() {
		return nil
	}
	if req.ctx.Err() != nil {
		req.tryRespondError(types.NewPDFError(types.ErrClientDropped, "", req.ctx.Err()))
		return nil
	}

	// Navigate to URL
	err := page.Navigate(request.URL)
	if err != nil {
		req.tryRespondError(types.NewPDFError(types.ErrGenerationFail, "", err))
		return nil
	}

	// Wait for element if specified
	waitSelector := request.WaitFor
	if waitSelector != "" {
		if req.hasResponded() {
			return nil
		}
		if req.ctx.Err() != nil {
			req.tryRespondError(types.NewPDFError(types.ErrClientDropped, "", req.ctx.Err()))
			return nil
		}

		// Create context with timeout for waiting
		var err error
		waitCtx, cancel := context.WithTimeout(req.ctx, 25*time.Second)
		defer cancel()

		if waitSelector[0] != '#' {
			_, err = page.Context(waitCtx).Element(waitSelector)
		} else {
			js := `
			(id) => new Promise((resolve) => {
				const e = document.getElementById(id);
				if (e) return resolve(true);
				const obs = new MutationObserver(recs => {
					for (const m of recs) {
						if (m.type === 'attributes' && m.attributeName === 'id' && m.target.id === id) {
							obs.disconnect(); return resolve(true);
						}
						if (m.type === 'childList') for (const n of m.addedNodes) {
							if (n.nodeType === 1) {
								if (n.id === id) { obs.disconnect(); return resolve(true); }
								const hit = n.querySelector && n.querySelector('#' + CSS.escape(id));
								if (hit) { obs.disconnect(); return resolve(true); }
							}
						}
					}
				});
				obs.observe(document, {subtree:true, childList:true, attributes:true, attributeFilter:['id']});
			})`
			_, err = page.Context(waitCtx).Evaluate(rod.Eval(js, waitSelector[1:]).ByPromise())
		}

		if err != nil {
			log.Printf("[%d, %s] failed to wait for element %q: %v", w.id, w.currentUrl, waitSelector, err)
			req.tryRespondError(types.NewPDFError(types.ErrElementNotReady, fmt.Sprintf("element %q", waitSelector), err))
			return nil
		}
	}

	if req.hasResponded() {
		return nil
	}
	if req.ctx.Err() != nil {
		req.tryRespondError(types.NewPDFError(types.ErrClientDropped, "", req.ctx.Err()))
		return nil
	}

	// Generate PDF
	scale := 1.0
	pdfOptions := &proto.PagePrintToPDF{
		PreferCSSPageSize:       true,
		Scale:                   &scale,
		GenerateTaggedPDF:       true,
		GenerateDocumentOutline: false,
	}

	if request.Options.PrintBackground {
		pdfOptions.PrintBackground = true
	}

	if request.Options.DisplayHeaderFooter {
		pdfOptions.DisplayHeaderFooter = true
		if request.Options.HeaderTemplate != "" {
			pdfOptions.HeaderTemplate = request.Options.HeaderTemplate
		}
		if request.Options.FooterTemplate != "" {
			pdfOptions.FooterTemplate = request.Options.FooterTemplate
		}
	}

	// Set margins if specified
	if request.Options.Margin.Top != "" {
		marginTop := convertMargin(request.Options.Margin.Top)
		pdfOptions.MarginTop = &marginTop
	}
	if request.Options.Margin.Right != "" {
		marginRight := convertMargin(request.Options.Margin.Right)
		pdfOptions.MarginRight = &marginRight
	}
	if request.Options.Margin.Bottom != "" {
		marginBottom := convertMargin(request.Options.Margin.Bottom)
		pdfOptions.MarginBottom = &marginBottom
	}
	if request.Options.Margin.Left != "" {
		marginLeft := convertMargin(request.Options.Margin.Left)
		pdfOptions.MarginLeft = &marginLeft
	}

	// pdfOptions.TransferMode = proto.PagePrintToPDFTransferModeReturnAsBase64
	// {
	// 	// DEBUG: JSON dump to logs
	// 	optionsJson, err := json.MarshalIndent(pdfOptions, "", "  ")
	// 	if err != nil {
	// 		log.Printf("[%d, %s] failed to marshal PDF options to JSON: %v", w.id, w.currentUrl, err)
	// 	} else {
	// 		log.Printf("[%d, %s] PDF options: %s", w.id, w.currentUrl, optionsJson)
	// 	}
	// }

	// NOTE: PDF is slightly larger in gorod because
	// `GenerateTaggedPDF` is omitted as empty during serialization
	res, err := pdfOptions.Call(page)
	if err != nil {
		req.tryRespondError(types.NewPDFError(types.ErrGenerationFail, "", err))
		return nil
	}
	// pdfBytes := make([]byte, base64.StdEncoding.DecodedLen(len(res.Data)))
	// n, err := base64.StdEncoding.Decode(pdfBytes, res.Data)
	// if err != nil {
	// 	req.tryRespondError(types.NewPDFError(types.ErrGenerationFail, "", err))
	// 	return nil
	// }
	// pdfBytes = pdfBytes[:n]
	pdfBytes := res.Data

	// We optimize for user latency, so we respond here as soon as we know we have a good PDF
	// The cleanup below happens independently in the background.
	// When it is complete, the worker can proceed to take user requests from the generator queue
	req.tryRespondOk(pdfBytes)

	return nil
}

func (w *browserWorker) cleanupBrowser(req *workerRequest) error {
	if req.cleanedUp {
		return nil
	}

	// Clear storage data for the origin using CDP command, same as ChromeDP
	err := proto.StorageClearDataForOrigin{
		Origin:       req.request.URL,
		StorageTypes: "all",
	}.Call(w.page)

	req.cleanedUp = err == nil
	return err
}

// mapRodError wraps raw rod errors while preserving our PDFErrors
func (w *browserWorker) mapRodError(err error) *types.PDFError {
	if err == nil {
		return nil
	}

	// Check if it's already our custom error type
	var pdfErr *types.PDFError
	if errors.As(err, &pdfErr) {
		return pdfErr
	}

	// Wrap other errors (including rod's internal wrapped errors)
	return types.NewPDFError(types.ErrUnhandledBrowserError, "", err)
}

func createBrowserLauncher() *launcher.Launcher {
	// Mirror ChromeDP's createBrowserOptions exactly
	// Start with defaults and modify to match ChromeDP's exact arguments
	l := launcher.New().
		Bin("/headless-shell/headless-shell").
		Headless(true).
		NoSandbox(true).
		UserDataDir("/tmp/browser-init")

	// Add ChromeDP-specific flags that are missing from Rod defaults
	l = l.Set("disable-font-subpixel-positioning").
		Set("font-render-hinting", "none").
		Set("hide-scrollbars").
		Set("mute-audio").
		Set("no-default-browser-check").
		Set("password-store", "basic").
		Set("disable-extensions").
		Set("safebrowsing-disable-auto-update")

	// Override disable-features to match ChromeDP exactly
	l = l.Delete("disable-features").
		Set("disable-features", "site-per-process,Translate,BlinkGenPropertyTrees")

	// Remove Rod-specific defaults that ChromeDP doesn't have
	l = l.Delete("disable-component-extensions-with-background-pages").
		Delete("disable-site-isolation-trials").
		Delete("no-startup-window")

	return l
}

func logArgs(launcher *launcher.Launcher) {
	args := launcher.FormatArgs()
	// Sort args
	sort.Strings(args)
	argsAsJson, err := json.MarshalIndent(args, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal browser args to JSON: %v", err)
	}
	log.Printf("Browser args: %v", string(argsAsJson))
}

// convertMargin converts margin strings like "0.75in" to float64 inches
func convertMargin(margin string) float64 {
	margin = strings.TrimSpace(margin)
	if len(margin) < 2 {
		return 0.75 // default
	}

	// Handle inches
	if strings.HasSuffix(margin, "in") {
		valueStr := strings.TrimSuffix(margin, "in")
		if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
			return value
		}
	}

	// Handle pixels (convert to inches, assuming 96 DPI)
	if strings.HasSuffix(margin, "px") {
		valueStr := strings.TrimSuffix(margin, "px")
		if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
			return value / 96.0 // Convert pixels to inches
		}
	}

	// Handle points (72 points = 1 inch)
	if strings.HasSuffix(margin, "pt") {
		valueStr := strings.TrimSuffix(margin, "pt")
		if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
			return value / 72.0 // Convert points to inches
		}
	}

	// Default fallback
	return 0.75
}
