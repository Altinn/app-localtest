package chromedp

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"altinn/pdf/internal/types"
)

type ChromeDP struct {
	workers        []*browserWorker
	queue          chan workerRequest
	wg             sync.WaitGroup
	browserVersion types.BrowserVersion
}

func New(chromePath string) (*ChromeDP, error) {
	workerCount := types.MaxConcurrency
	fmt.Printf("Starting ChromeDP with %d browser workers\n", workerCount)

	generator := &ChromeDP{
		workers: make([]*browserWorker, 0, workerCount),
		queue:   make(chan workerRequest, workerCount*2),
	}

	go func() {
		fmt.Printf("Initializing ChromeDP\n")

		// Get browser version info using the same Chrome path
		opts, err := createBrowserOptions(chromePath, path.Join(os.TempDir(), "browser-init"))
		if err != nil {
			log.Fatalf("Failed to create browser options: %v", err)
		}

		allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
		tempCtx, cancel2 := chromedp.NewContext(allocCtx)
		defer func() {
			cancel2()
			cancel()
		}()

		var product, revision, protocolVersion, userAgent, jsVersion string
		err = chromedp.Run(tempCtx, chromedp.ActionFunc(func(ctx context.Context) error {
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
			fmt.Printf("Starting browser worker %d\n", i)
			worker, err := newBrowserWorker(i, chromePath)
			if err != nil {
				log.Fatalf("Failed to create browserworker %d: %v", i, err)
			}

			generator.workers = append(generator.workers, worker)
			generator.wg.Add(1)

			go func(i int) {
				fmt.Printf("Browser worker %d started successfully\n", i)
				defer generator.wg.Done()
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
	id     int
	ctx    context.Context
	cancel context.CancelFunc
}

func newBrowserWorker(id int, chromePath string) (*browserWorker, error) {
	opts, err := createBrowserOptions(chromePath, path.Join(os.TempDir(), fmt.Sprintf("browser-%d", id)))
	if err != nil {
		return nil, err
	}

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel2 := chromedp.NewContext(allocCtx, chromedp.WithBrowserOption(chromedp.WithBrowserLogf(func(format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		if strings.Contains(msg, "could not unmarshal event") {
			// Comes from (breaking) changes in CDP protocol (library is tested against different versions of Chrome)
			// We are keeping browser version stable for now so just ignore as long as everything works
			return
		}
		fmt.Printf("%s", "[Browser] "+msg+"\n")
	})))

	combinedCancel := func() {
		cancel2()
		cancel()
	}

	worker := &browserWorker{
		id:     id,
		ctx:    ctx,
		cancel: combinedCancel,
	}

	if err := chromedp.Run(ctx); err != nil {
		combinedCancel()
		return nil, fmt.Errorf("failed to initialize browser worker %d: %v", id, err)
	}

	fmt.Printf("Browser worker %d initialized successfully\n", id)
	return worker, nil
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
			w.handleRequest(&req)
			if !req.hasResponded() {
				log.Fatalf("Worker %d did not respond to request\n", w.id)
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
			fmt.Println("Recovered from panic:", r)
			req.tryRespondError(fmt.Errorf("panic handling request %s: %v", req.request.URL, r))
		}
	}()

	start := time.Now()

	err := chromedp.Run(w.ctx, w.generatePdf(req))

	duration := time.Since(start)
	fmt.Printf("Worker %d completed PDF request for URL: %s in %.2f seconds\n", w.id, req.request.URL, duration.Seconds())

	if err != nil {
		req.tryRespondError(err)
	}
}

func (w *browserWorker) generatePdf(req *workerRequest) chromedp.Tasks {
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
	if waitSelector == "" {
		waitSelector = "#readyForPrint"
	}
	tasks = append(tasks, chromedp.WaitReady(waitSelector, chromedp.ByID))

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

	// Clear cookies
	tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.ClearBrowserCookies().Do(ctx)
	}))

	return tasks
}

func createBrowserOptions(chromePath string, userDataDir string) ([]chromedp.ExecAllocatorOption, error) {
	if err := os.MkdirAll(userDataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create user data dir: %v", err)
	}

	return []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(chromePath),
		chromedp.Flag("headless", "new"),
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("mute-audio", true),
		chromedp.UserDataDir(userDataDir),
		chromedp.NoSandbox,
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.NoFirstRun,
		chromedp.Flag("enable-logging", true),
		chromedp.Flag("v1", "1"),
	}, nil
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
