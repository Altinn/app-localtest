package chromedp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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

	go func() {
		fmt.Printf("Initializing ChromeDP\n")

		initCtx, cancel := chromedp.NewContext(context.Background())
		defer func() {
			cancel()
		}()

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

		for i := 0; i < workerCount; i++ {
			generator.wg.Add(1)
			go func(i int) {
				defer generator.wg.Done()
				fmt.Printf("Starting browser worker %d\n", i)
				worker, err := newBrowserWorker(i)
				if err != nil {
					log.Fatalf("Failed to create browserworker %d: %v", i, err)
				}

				generator.workers[i] = worker
				fmt.Printf("Browser worker %d started successfully\n", i)
				worker.run(generator.queue)
				fmt.Printf("Browser worker %d terminated\n", i)
			}(i)
		}
	}()

	return generator, nil
}

func (g *ChromeDP) Generate(ctx context.Context, request types.PdfRequest) (*types.PdfResult, error) {
	responder := make(chan workerResponse, 1)
	req := workerRequest{
		request:   request,
		responder: responder,
		ctx:       ctx,
	}

	select {
	case g.queue <- req:
		break
	case <-time.After(5 * time.Second):
		fmt.Printf("Request queue full, rejecting request for URL: %s (buffer: %d/%d)\n", request.URL, len(g.queue), cap(g.queue))
		return nil, fmt.Errorf("pool is busy, request timeout")
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
	case <-time.After(30 * time.Second):
		fmt.Printf("Client timeout waiting for PDF generation (30s) for URL: %s - abandoning request\n", request.URL)
		return nil, fmt.Errorf("PDF generation timeout")
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
}

func (r *workerRequest) respondOk(data []byte) {
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

func (r *workerRequest) tryRespondError(err error) {
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
	Error error
}

type browserWorker struct {
	id            int
	ctx           context.Context
	cancel        context.CancelFunc
	errors        []string
	readyForPrint chan struct{}
	currentUrl    string
}

const readyForPrintCbName = "altinnStudioAppReadyForPrintCb"

func newBrowserWorker(id int) (*browserWorker, error) {
	ctx, cancel := chromedp.NewContext(context.Background(), chromedp.WithBrowserOption(chromedp.WithBrowserLogf(func(format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		if strings.Contains(msg, "could not unmarshal event") {
			// Comes from (breaking) changes in CDP protocol (library is tested against different versions of Chrome)
			// We are keeping browser version stable for now so just ignore as long as everything works
			return
		}
		fmt.Printf("%s", "[Browser] "+msg+"\n")
	})))

	w := &browserWorker{
		id:            id,
		ctx:           ctx,
		cancel:        cancel,
		errors:        make([]string, 32),
		readyForPrint: make(chan struct{}, 1),
		currentUrl:    "",
	}

	if err := chromedp.Run(w.ctx, chromedp.Tasks{
		runtime.AddBinding(readyForPrintCbName),
	}); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize browser worker %d: %v", id, err)
	}

	chromedp.ListenTarget(w.ctx, func(ev interface{}) {
		if ev, ok := ev.(*cdplog.EventEntryAdded); ok {
			if ev.Entry.Level == cdplog.LevelError {
				errorJson, err := json.MarshalIndent(ev.Entry, "", "  ")
				if err != nil {
					errorMsgFormat := "%s - %s"
					errorMsg := fmt.Sprintf(errorMsgFormat, ev.Entry.URL, ev.Entry.Text)
					w.errors = append(w.errors, errorMsg)
					fmt.Printf("[%d, %s] console error: %s\n", w.id, w.currentUrl, errorMsg)
				} else {
					errorJsonStr := string(errorJson)
					w.errors = append(w.errors, errorJsonStr)
					fmt.Printf("[%d, %s] console error: %s\n", w.id, w.currentUrl, "")
				}
			}
		}

		if ev, ok := ev.(*runtime.EventConsoleAPICalled); ok {
			if ev.Type == "error" {
				errorJson, err := json.MarshalIndent(ev, "", "  ")
				if err != nil {
					errorMsgFormat := "%s - %s"
					errorMsg := fmt.Sprintf(errorMsgFormat, ev.Type, ev.Context)
					w.errors = append(w.errors, errorMsg)
					fmt.Printf("[%d, %s] console error: %s\n", w.id, w.currentUrl, errorMsg)
				} else {
					errorJsonStr := string(errorJson)
					w.errors = append(w.errors, errorJsonStr)
					fmt.Printf("[%d, %s] console error: %s\n", w.id, w.currentUrl, "")
				}
			}
		}

		if ev, ok := ev.(*runtime.EventBindingCalled); ok {
			fmt.Printf("[%d, %s] received event binding call: %s\n", id, w.currentUrl, ev.Name)
			if ev.Name == readyForPrintCbName {
				select {
				case w.readyForPrint <- struct{}{}:
					break
				default:
					fmt.Printf("[%d, %s] readyForPrint channel full, ignoring\n", w.id, w.currentUrl)
				}
			}
		}
	})

	fmt.Printf("Browser worker %d initialized successfully\n", id)
	return w, nil
}

func (w *browserWorker) run(requestCh <-chan workerRequest) {
	defer w.cancel()

	for {
		select {
		case req, ok := <-requestCh:
			if !ok {
				fmt.Printf("Worker %d shutting down\n", w.id)
				return
			}
			w.currentUrl = req.request.URL
			w.handleRequest(&req)
			w.currentUrl = ""
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
			req.tryRespondError(fmt.Errorf("panic handling request %s: %v", req.request.URL, r))
		}
	}()

	w.errors = w.errors[:0] // TODO: include errors in response?

	// Drain any stale readyForPrint signals from previous runs
	select {
	case <-w.readyForPrint:
		break
	default:
		break
	}

	start := time.Now()

	erroredWaiting := false

	err := chromedp.Run(w.ctx, w.generatePdf(req, &erroredWaiting))

	if erroredWaiting {
		fmt.Printf("[%d, %s] encountered errors while waiting for elements: %s\n", w.id, w.currentUrl, req.request.WaitFor)
	}

	duration := time.Since(start)
	fmt.Printf("[%d, %s] completed PDF request for URL: %s in %.2f seconds\n", w.id, w.currentUrl, req.request.URL, duration.Seconds())

	if err != nil {
		req.tryRespondError(err)
	}
}

func (w *browserWorker) generatePdf(req *workerRequest, erroredWaiting *bool) chromedp.Tasks {
	tasks := chromedp.Tasks{}

	// Set cookies from request
	request := req.request
	for _, cookie := range request.Cookies {
		tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
			sameSite := network.CookieSameSiteLax
			switch cookie.SameSite {
			case "Strict":
				sameSite = network.CookieSameSiteStrict
			case "None":
				sameSite = network.CookieSameSiteNone
			}
			return network.SetCookie(cookie.Name, cookie.Value).
				WithDomain(cookie.Domain).
				WithPath("/").
				WithSecure(false).
				WithHTTPOnly(false).
				WithSameSite(sameSite).
				Do(ctx)
		}))
	}

	// Navigate to URL
	tasks = append(tasks, chromedp.Navigate(request.URL))

	// Wait for element if specified
	waitSelector := request.WaitFor
	if waitSelector != "" {
		tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			err := chromedp.WaitReady(waitSelector, chromedp.ByQuery).Do(ctx)
			if err != nil {
				*erroredWaiting = true
				log.Printf("[%d, %s] failed to wait for element %q: %v", w.id, w.currentUrl, waitSelector, err)
			}
			return err
		}))
	} else {
		tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			select {
			case <-w.readyForPrint:
				break
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		}))
	}

	// Generate PDF with options
	tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
		pdfParams := page.PrintToPDF()

		// Apply options
		if request.Options.PrintBackground {
			pdfParams = pdfParams.WithPrintBackground(true)
		}

		if request.Options.DisplayHeaderFooter {
			pdfParams = pdfParams.WithDisplayHeaderFooter(true)
			if request.Options.HeaderTemplate != "" {
				pdfParams = pdfParams.WithHeaderTemplate(request.Options.HeaderTemplate)
			}
			if request.Options.FooterTemplate != "" {
				pdfParams = pdfParams.WithFooterTemplate(request.Options.FooterTemplate)
			}
		}

		// Set margins if specified
		if request.Options.Margin.Top != "" {
			pdfParams = pdfParams.WithMarginTop(convertMargin(request.Options.Margin.Top))
		}
		if request.Options.Margin.Right != "" {
			pdfParams = pdfParams.WithMarginRight(convertMargin(request.Options.Margin.Right))
		}
		if request.Options.Margin.Bottom != "" {
			pdfParams = pdfParams.WithMarginBottom(convertMargin(request.Options.Margin.Bottom))
		}
		if request.Options.Margin.Left != "" {
			pdfParams = pdfParams.WithMarginLeft(convertMargin(request.Options.Margin.Left))
		}

		buf, _, err := pdfParams.Do(ctx)
		if err != nil {
			return err
		}

		req.respondOk(buf)
		return nil
	}))

	// Navigate back to default
	tasks = append(tasks, chromedp.Navigate("about:blank"))

	// Clear origin data (storage, cookies, etc.)
	tasks = append(tasks, storage.ClearDataForOrigin(request.URL, "all"))

	return tasks
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
