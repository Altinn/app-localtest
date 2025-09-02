package chromedp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/browser"
	cdplog "github.com/chromedp/cdproto/log"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"

	"altinn/pdf/internal/types"
)

type ChromeDP struct {
	workers        []*browserWorker
	queue          chan workerRequest
	wg             sync.WaitGroup
	browserVersion types.BrowserVersion
}

func New() (*ChromeDP, error) {
	workerCount := types.MaxConcurrency
	fmt.Printf("Starting ChromeDP with %d browser workers\n", workerCount)

	generator := &ChromeDP{
		workers: make([]*browserWorker, workerCount),
		queue:   make(chan workerRequest, workerCount*2),
	}

	// We only need the queue to be initialized before returning the generator to main
	// We can initialize the workers asynchronously, and they will start
	// consuming requests from the queue.
	// We should be reasonably sure that the workers are quick to start (<1s, otherwise we might get issues)
	go func() {
		fmt.Printf("Initializing ChromeDP\n")

		opts := createBrowserOptions()
		opts = append(opts, logArgs())
		allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
		defer allocCancel()
		initCtx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
		defer cancel()

		var product, revision, protocolVersion, userAgent, jsVersion string
		err := chromedp.Run(initCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			product, protocolVersion, revision, userAgent, jsVersion, err = browser.GetVersion().Do(ctx)
			return err
		}))
		if err != nil {
			log.Fatalf("Failed to get browser version: %v", err)
		}

		generator.browserVersion = types.BrowserVersion{
			Product:         product,
			ProtocolVersion: protocolVersion,
			Revision:        revision,
			UserAgent:       userAgent,
			JSVersion:       jsVersion,
		}

		fmt.Printf("Chrome version: %s (revision: %s, protocol: %s)\n", product, revision, protocolVersion)

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

func (g *ChromeDP) Generate(ctx context.Context, request types.PdfRequest) (*types.PdfResult, *types.PDFError) {
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

func (g *ChromeDP) Close() error {
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
	id          int
	ctx         context.Context
	cancel      context.CancelFunc
	allocCtx    context.Context
	allocCancel context.CancelFunc
	// errors      []string
	currentUrl string
}

func newBrowserWorker(id int) *browserWorker {
	opts := createBrowserOptions()
	// Override user data directory for this worker
	opts = append(opts, chromedp.UserDataDir(fmt.Sprintf("/tmp/browser-%d", id)))
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithBrowserOption(chromedp.WithBrowserLogf(func(format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		// We had some log noise due to differences in CDP protocol versions between the installed browser and the version of chromedp
		// but this was only back when we used the old version of browserless container and needed common ground for testing

		// if strings.Contains(msg, "could not unmarshal event") {
		// 	// Comes from (breaking) changes in CDP protocol (library is tested against different versions of Chrome)
		// 	// We are keeping browser version stable for now so just ignore as long as everything works
		// 	return
		// }
		fmt.Printf("%s", "[Browser] "+msg+"\n")
	})))

	w := &browserWorker{
		id:          id,
		ctx:         ctx,
		cancel:      cancel,
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		// errors:      make([]string, 32),
		currentUrl: "",
	}

	// Possible improvement:
	//   Right now we wait for #readyForPrint element to be ready in the DOM
	//   There are no awesome ways of waiting for this element to show up.
	//   * Poll querying the DOM
	//   * requestAnimationFrame (RAF) polling, which is what puppeteer does in some cases
	//   What we could consider for the future is not using the DOM for communicating this at all
	//   and rather call into exposed functions through the CDP `Runtime.addBinding` command.
	//   Using this approach a function would become available on the `windows` object
	//   which the frontend could call (instead of appending to the DOM), or alternatively
	//   have the frontend register a `MutationObserver` or some other mechanism to react to the DOM change
	//   which would be more efficient than communicating over CDP/websocket.
	//   This was tested but didn't improve perf to a significant degree, so was left out initially (would require app changes)
	//   See outline of implementation:
	//
	//   chromedp.Tasks{
	//   	runtime.AddBinding("altinnStudioAppReadyForPrintCb"),
	//   }
	//
	//   From JS we can call this function:
	//
	//   if (window.hasOwnProperty('altinnStudioAppReadyForPrintCb')) {
	//   	try {
	//   		const result = window.altinnStudioAppReadyForPrintCb('ready');
	//   	} catch (err) {
	//   		console.error('Error occurred while calling callback:', err);
	//   	}
	//   }
	//
	//   On the Go side, we would listen for the call:
	//
	//   chromedp.ListenTarget(w.ctx, func(ev interface{}) {
	//   	if ev, ok := ev.(*runtime.EventBindingCalled); ok {
	//   		fmt.Printf("[%d, %s] received event binding call: %s\n", id, w.currentUrl, ev.Name)
	//   		if ev.Name == readyForPrintCbName {
	//   			select {
	//   			case w.readyForPrint <- struct{}{}:
	//   				break
	//   			default:
	//   				fmt.Printf("[%d, %s] readyForPrint channel full, ignoring\n", w.id, w.currentUrl)
	//   			}
	//   		}
	//   	}
	//   })
	//
	//   The `readyForPrint` here is a one shot channel that is consumed from
	//   blocking until we run the PDF print command:
	//
	//   tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
	//   	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	//   	defer cancel()
	//   	select {
	//   	case <-w.readyForPrint:
	//   		break
	//   	case <-ctx.Done():
	//   		return ctx.Err()
	//   	}
	//   	return nil
	//   }))
	//
	//   Now, print is called
	//

	if err := chromedp.Run(w.ctx); err != nil {
		cancel()
		allocCancel()
		log.Fatalf("Browser worker %d failed to initialize: %v", id, err)
	}

	chromedp.ListenTarget(w.ctx, func(ev interface{}) {
		// These are internal log events only
		if ev, ok := ev.(*cdplog.EventEntryAdded); ok {
			if ev.Entry.Level == cdplog.LevelError {
				errorJson, err := json.MarshalIndent(ev.Entry, "", "  ")
				if err != nil {
					errorMsgFormat := "%s - %s"
					errorMsg := fmt.Sprintf(errorMsgFormat, ev.Entry.URL, ev.Entry.Text)
					// w.errors = append(w.errors, errorMsg)
					fmt.Printf("[%d, %s] console error: %s\n", w.id, w.currentUrl, errorMsg)
				} else {
					errorJsonStr := string(errorJson)
					// w.errors = append(w.errors, errorJsonStr)
					fmt.Printf("[%d, %s] console error: %s\n", w.id, w.currentUrl, errorJsonStr)
				}
			}
		}

		// `console.error` also show up in these ones (not the above ones)
		if ev, ok := ev.(*runtime.EventConsoleAPICalled); ok {
			if ev.Type == "error" {
				errorJson, err := json.MarshalIndent(ev, "", "  ")
				if err != nil {
					errorMsgFormat := "%s - %s"
					errorMsg := fmt.Sprintf(errorMsgFormat, ev.Type, ev.Context)
					// w.errors = append(w.errors, errorMsg)
					fmt.Printf("[%d, %s] console error: %s\n", w.id, w.currentUrl, errorMsg)
				} else {
					errorJsonStr := string(errorJson)
					// w.errors = append(w.errors, errorJsonStr)
					fmt.Printf("[%d, %s] console error: %s\n", w.id, w.currentUrl, errorJsonStr)
				}
			}
		}
	})

	fmt.Printf("Browser worker %d initialized successfully\n", id)
	return w
}

func (w *browserWorker) run(queue <-chan workerRequest) {
	defer w.allocCancel()
	defer w.cancel()

	for {
		select {
		case req, ok := <-queue:
			if !ok {
				fmt.Printf("Worker %d shutting down\n", w.id)
				return
			}
			w.currentUrl = req.request.URL
			w.handleRequest(&req)
			w.currentUrl = ""
			// It's important that we always respond, otherwise we leave
			// requests hanging and timing out for no reason
			if !req.hasResponded() {
				log.Fatalf("[%d, %s] did not respond to request\n", w.id, w.currentUrl)
			}
		case <-w.ctx.Done():
			fmt.Printf("Worker %d shutting down\n", w.id)
			return
		}
	}
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

	// TODO: include errors in response as diagnostics?
	// metrics better suited?
	// w.errors = w.errors[:0]

	start := time.Now()

	err := chromedp.Run(w.ctx, w.generatePdf(req))
	// Immediately after running we should verify that we've ran the cleanup tasks
	// If we haven't, theres a bug and we should just crash hard
	if !req.cleanedUp {
		log.Printf("[%d, %s] failed to cleanup storage, retrying...", w.id, w.currentUrl)
		for i := range 3 {
			err := chromedp.Run(w.ctx, cleanupTask(req))
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

	duration := time.Since(start)
	fmt.Printf("[%d, %s] completed PDF request in %.2f seconds\n", w.id, w.currentUrl, duration.Seconds())

	if err != nil {
		req.tryRespondError(w.mapChromedpError(err))
	}
}

func (w *browserWorker) generatePdf(req *workerRequest) chromedp.Tasks {
	tasks := chromedp.Tasks{}

	request := req.request
	for _, cookie := range request.Cookies {
		tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
			if req.ctx.Err() != nil {
				// It's OK to just return here because we haven't done anything with
				// the users input yet.
				return types.NewPDFError(types.ErrClientDropped, "", req.ctx.Err())
			}

			sameSite := network.CookieSameSiteLax
			switch cookie.SameSite {
			case "Strict":
				sameSite = network.CookieSameSiteStrict
			case "None":
				sameSite = network.CookieSameSiteNone
			}
			err := network.SetCookie(cookie.Name, cookie.Value).
				WithDomain(cookie.Domain).
				WithPath("/").
				WithSecure(false).
				WithHTTPOnly(false).
				WithSameSite(sameSite).
				Do(ctx)
			if err != nil {
				req.tryRespondError(types.NewPDFError(types.ErrSetCookieFail, "", err))
			}
			return nil
		}))
	}

	tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
		// Now we have potentially set cookies, so it is no longer safe
		// to return errors from tasks because then chromedp will shortcircuit and
		// not execute the cleanup tasks at the end of the task slice. See below
		if req.hasResponded() {
			return nil
		}
		if req.ctx.Err() != nil {
			req.tryRespondError(types.NewPDFError(types.ErrClientDropped, "", req.ctx.Err()))
			return nil
		}
		err := chromedp.Navigate(request.URL).Do(ctx)
		if err != nil {
			req.tryRespondError(types.NewPDFError(types.ErrGenerationFail, "", err))
		}
		return nil
	}))

	waitSelector := request.WaitFor
	if waitSelector != "" {
		tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
			if req.hasResponded() {
				return nil
			}
			if req.ctx.Err() != nil {
				req.tryRespondError(types.NewPDFError(types.ErrClientDropped, "", req.ctx.Err()))
				return nil
			}

			// TODO: should we fail early here if we discover errors?
			// update frontend to communicate significant errors?
			// Some errors are not significant, e.g. failing requests
			// (e.g. user profile can give 400 for service owner tokens, but the frontend handles that)
			ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
			defer cancel()
			err := chromedp.WaitReady(waitSelector, chromedp.ByQuery).Do(ctx)
			if err != nil {
				log.Printf("[%d, %s] failed to wait for element %q: %v", w.id, w.currentUrl, waitSelector, err)
				req.tryRespondError(types.NewPDFError(types.ErrElementNotReady, fmt.Sprintf("element %q", waitSelector), err))
			}
			return nil
		}))
	}

	tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
		if req.hasResponded() {
			return nil
		}
		if req.ctx.Err() != nil {
			req.tryRespondError(types.NewPDFError(types.ErrClientDropped, "", req.ctx.Err()))
			return nil
		}

		pdfOptions := page.PrintToPDF().
			WithPreferCSSPageSize(true).
			WithScale(1).
			WithGenerateTaggedPDF(true).
			WithGenerateDocumentOutline(false)

		if request.Options.PrintBackground {
			pdfOptions = pdfOptions.WithPrintBackground(true)
		}

		if request.Options.DisplayHeaderFooter {
			pdfOptions = pdfOptions.WithDisplayHeaderFooter(true)
			if request.Options.HeaderTemplate != "" {
				pdfOptions = pdfOptions.WithHeaderTemplate(request.Options.HeaderTemplate)
			}
			if request.Options.FooterTemplate != "" {
				pdfOptions = pdfOptions.WithFooterTemplate(request.Options.FooterTemplate)
			}
		}

		// Set margins if specified
		if request.Options.Margin.Top != "" {
			pdfOptions = pdfOptions.WithMarginTop(convertMargin(request.Options.Margin.Top))
		}
		if request.Options.Margin.Right != "" {
			pdfOptions = pdfOptions.WithMarginRight(convertMargin(request.Options.Margin.Right))
		}
		if request.Options.Margin.Bottom != "" {
			pdfOptions = pdfOptions.WithMarginBottom(convertMargin(request.Options.Margin.Bottom))
		}
		if request.Options.Margin.Left != "" {
			pdfOptions = pdfOptions.WithMarginLeft(convertMargin(request.Options.Margin.Left))
		}

		// {
		// 	// DEBUG: JSON dump to logs
		// 	optionsJson, err := json.MarshalIndent(pdfOptions, "", "  ")
		// 	if err != nil {
		// 		log.Printf("[%d, %s] failed to marshal PDF options to JSON: %v", w.id, w.currentUrl, err)
		// 	} else {
		// 		log.Printf("[%d, %s] PDF options: %s", w.id, w.currentUrl, optionsJson)
		// 	}
		// }
		buf, _, err := pdfOptions.Do(ctx)
		if err != nil {
			req.tryRespondError(types.NewPDFError(types.ErrGenerationFail, "", err))
			return nil
		}

		// We optimize for user latency, so we respond here as soon as we know we have a good PDF
		// The cleanup below happens independently in the background.
		// When it is complete, the worker can proceed to take user requests from the generator queue
		req.tryRespondOk(buf)
		return nil
	}))

	// Navigate back to default
	// It's important that this is here, otherwise Chrome isn't really going
	// to navigate if the current request is the same URL as the last request
	// (unlikely, but can happen during tests or due to client retries)
	tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
		err := chromedp.Navigate("about:blank").Do(ctx)
		if err != nil {
			log.Printf("[%d, %s] failed to navigate out of url: %v", w.id, w.currentUrl, err)
		}
		return nil
	}))

	// TODO: which resources don't have to be cleared? Would be nice to cache static assets
	// one way to find out would be to analyze the user-data-dir in the container before/after clearing data
	// and running some experiments with different storage types left out
	tasks = append(tasks, cleanupTask(req))

	return tasks
}

func cleanupTask(req *workerRequest) chromedp.ActionFunc {
	request := &req.request
	return chromedp.ActionFunc(func(ctx context.Context) error {
		if req.cleanedUp {
			return nil
		}
		err := storage.ClearDataForOrigin(request.URL, "all").Do(ctx)
		req.cleanedUp = err == nil
		return err
	})
}

func createBrowserOptions() []chromedp.ExecAllocatorOption {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		// Text layout is slightly off without these options..
		// Discovered while moving from old version of browserless container
		chromedp.Flag("disable-font-subpixel-positioning", true),
		chromedp.Flag("font-render-hinting", "none"),
		// Set user data directory for init browser
		chromedp.UserDataDir("/tmp/browser-init"),
	)
	return opts
}

func logArgs() chromedp.ExecAllocatorOption {
	return chromedp.ModifyCmdFunc(func(cmd *exec.Cmd) {
		// Make copy of args
		args := make([]string, len(cmd.Args))
		copy(args, cmd.Args)
		sort.Strings(args)
		argsAsJson, err := json.MarshalIndent(args, "", "  ")
		if err != nil {
			log.Fatalf("Failed to marshal browser args to JSON: %v", err)
		}
		log.Printf("Browser args: %v", string(argsAsJson))
	})
}

// mapChromedpError wraps raw chromedp errors while preserving our PDFErrors
func (w *browserWorker) mapChromedpError(err error) *types.PDFError {
	if err == nil {
		return nil
	}

	// Check if it's already our custom error type
	var pdfErr *types.PDFError
	if errors.As(err, &pdfErr) {
		return pdfErr
	}

	// Wrap other errors (including chromedp's internal wrapped errors)
	return types.NewPDFError(types.ErrUnhandledBrowserError, "", err)
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
