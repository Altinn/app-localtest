use crate::pdf::{PdfResult, RequestInfo};
use crate::{PdfGenerator, PdfRequest, pdf};
use async_channel::{Receiver, Sender};
use async_trait::async_trait;
use chromiumoxide::Page;
use chromiumoxide::browser::{Browser, BrowserConfig, HeadlessMode};
use chromiumoxide::cdp::browser_protocol::network::{CookieParam, CookieSameSite};
use chromiumoxide::cdp::browser_protocol::page::PrintToPdfParams;
use chromiumoxide::error::CdpError;
use futures::{FutureExt, StreamExt};
use std::panic::AssertUnwindSafe;
use std::path::Path;
use std::sync::OnceLock;
use std::time::Duration;
use tokio::runtime::Handle;
use tokio::sync::oneshot;
use tokio::time::timeout;

#[derive(Debug, Clone)]
pub enum ChromiumOxideError {
    BrowserError(String),
    NavigationError(String),
    PdfGenerationError(String),
    TimeoutError(String),
}

impl std::fmt::Display for ChromiumOxideError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ChromiumOxideError::BrowserError(msg) => write!(f, "Browser error: {}", msg),
            ChromiumOxideError::NavigationError(msg) => write!(f, "Navigation error: {}", msg),
            ChromiumOxideError::PdfGenerationError(msg) => {
                write!(f, "PDF generation error: {}", msg)
            }
            ChromiumOxideError::TimeoutError(msg) => write!(f, "Timeout error: {}", msg),
        }
    }
}

impl std::error::Error for ChromiumOxideError {}

static BROWSER_VERSION_CELL: OnceLock<pdf::BrowserVersion> = OnceLock::new();

pub struct ChromiumOxide {
    queue: Sender<WorkerRequest>,
}

impl ChromiumOxide {
    pub async fn new(chrome_path: &Path) -> Result<Self, ChromiumOxideError> {
        let worker_count = pdf::MAX_CONCURRENCY;
        println!(
            "Starting ChromiumOxide with {} browser workers",
            worker_count
        );

        let (tx, rx) = async_channel::bounded::<WorkerRequest>(worker_count * 2);
        let generator = Self { queue: tx };

        let chrome_path = chrome_path.to_path_buf();
        _ = tokio::spawn(async move {
            println!("Initializing ChromiumOxide");

            let init_user_data_dir = std::env::temp_dir().join("browser-init");
            let config =
                create_browser_config(&chrome_path, &init_user_data_dir).unwrap_or_else(|e| {
                    eprintln!("Error during init: {}", e);
                    std::process::exit(1);
                });
            let (mut init_browser, mut init_handler) = Browser::launch(config)
                .await
                .map_err(|e| {
                    ChromiumOxideError::BrowserError(format!(
                        "Failed to launch init browser: {}",
                        e
                    ))
                })
                .unwrap_or_else(|e| {
                    eprintln!("Error during init: {}", e);
                    std::process::exit(1);
                });

            // Wait for handler to finish
            tokio::spawn(async move { while let Some(_) = init_handler.next().await {} });

            let version = init_browser
                .version()
                .await
                .map_err(|e| {
                    ChromiumOxideError::BrowserError(format!(
                        "Failed to get browser version: {}",
                        e
                    ))
                })
                .unwrap_or_else(|e| {
                    eprintln!("Error during init: {}", e);
                    std::process::exit(1);
                });

            println!(
                "Chrome version: {} (revision: {}, protocol: {})",
                version.product, version.revision, version.protocol_version
            );

            // Close init browser
            init_browser
                .close()
                .await
                .map_err(|e| {
                    ChromiumOxideError::BrowserError(format!("Failed to close init browser: {}", e))
                })
                .unwrap_or_else(|e| {
                    eprintln!("Error during init: {}", e);
                    std::process::exit(1);
                });
            init_browser
                .wait()
                .await
                .map_err(|e| {
                    ChromiumOxideError::BrowserError(format!(
                        "Failed to wait for init browser to close: {}",
                        e
                    ))
                })
                .unwrap_or_else(|e| {
                    eprintln!("Error during init: {}", e);
                    std::process::exit(1);
                });

            // Convert version strings to static by leaking them
            let browser_version = pdf::BrowserVersion {
                protocol_version: Box::leak(version.protocol_version.into_boxed_str()),
                product: Box::leak(version.product.into_boxed_str()),
                revision: Box::leak(version.revision.into_boxed_str()),
                user_agent: Box::leak(version.user_agent.into_boxed_str()),
                js_version: Box::leak(version.js_version.into_boxed_str()),
            };
            BROWSER_VERSION_CELL.set(browser_version).unwrap();

            // Create multiple workers
            for worker_id in 0..worker_count {
                let worker_rx = rx.clone();
                let chrome_path = chrome_path.to_path_buf();

                tokio::spawn(async move {
                    println!("Starting browser worker {}", worker_id);
                    match BrowserWorker::new(worker_id, &chrome_path).await {
                        Ok(worker) => {
                            println!("Browser worker {} started successfully", worker_id);
                            worker.run(worker_rx).await;
                            println!("Browser worker {} terminated", worker_id);
                        }
                        Err(e) => {
                            eprintln!("Failed to create browser worker {}: {}", worker_id, e);
                        }
                    }
                });
            }
        });

        Ok(generator)
    }
}

#[async_trait]
impl PdfGenerator for ChromiumOxide {
    async fn generate(
        &self,
        request: PdfRequest,
    ) -> Result<PdfResult, Box<dyn std::error::Error + Send + Sync>> {
        let (tx, rx) = oneshot::channel::<Result<WorkerResponse, ChromiumOxideError>>();
        let worker_request = WorkerRequest {
            request,
            responder: WorkerRequestResponder { inner: Some(tx) },
        };

        // Send request to worker pool
        self.queue.send(worker_request).await.map_err(|e| {
            ChromiumOxideError::BrowserError(format!("Failed to send request to worker: {}", e))
        })?;

        // Wait for response from worker
        let result = rx.await.map_err(|e| {
            ChromiumOxideError::BrowserError(format!(
                "Failed to receive response from worker: {}",
                e
            ))
        })?;

        let worker_response = result.map_err(|e| {
            ChromiumOxideError::BrowserError(format!("Failed to process worker response: {}", e))
        })?;

        let browser_version = BROWSER_VERSION_CELL.get().ok_or_else(|| {
            ChromiumOxideError::BrowserError("Browser version not initialized".to_string())
        })?;
        Ok(PdfResult {
            data: worker_response.data,
            browser: browser_version.clone(),
            request_info: worker_response.request_info,
        })
    }
}

impl Drop for BrowserWorker {
    fn drop(&mut self) {
        Handle::current().block_on(async move {
            self.browser.close().await.unwrap();
        });
    }
}

struct WorkerResponse {
    data: Vec<u8>,
    request_info: RequestInfo,
}

struct WorkerRequest {
    request: PdfRequest,
    responder: WorkerRequestResponder,
}

struct WorkerRequestResponder {
    inner: Option<oneshot::Sender<Result<WorkerResponse, ChromiumOxideError>>>,
}

impl WorkerRequestResponder {
    fn try_respond_ok(&mut self, data: Vec<u8>, request_info: RequestInfo) {
        if let Some(inner) = self.inner.take() {
            let _ = inner.send(Ok(WorkerResponse { data, request_info }));
        }
    }

    fn try_respond_err(&mut self, error: ChromiumOxideError) {
        if let Some(inner) = self.inner.take() {
            let _ = inner.send(Err(error));
        }
    }

    fn has_responded(&self) -> bool {
        self.inner.is_none()
    }
}

struct BrowserWorker {
    id: usize,
    browser: Browser,
}

impl BrowserWorker {
    async fn new(worker_id: usize, chrome_path: &Path) -> Result<Self, ChromiumOxideError> {
        // Create unique temp directory for each worker to avoid SingletonLock conflicts
        let user_data_dir = std::env::temp_dir().join(format!("browser-{}", worker_id));
        let config = create_browser_config(chrome_path, &user_data_dir)?;
        let (browser, mut handler) = Browser::launch(config).await.map_err(|e| {
            ChromiumOxideError::BrowserError(format!("Failed to launch browser: {}", e))
        })?;

        tokio::spawn(async move {
            while let Some(result) = handler.next().await {
                if let Err(err) = result
                    && !matches!(err, CdpError::Serde(_))
                {
                    eprintln!("Browser handler error: {:?}", err);
                }
            }
        });

        Ok(Self {
            id: worker_id,
            browser,
        })
    }

    async fn run(mut self, queue: Receiver<WorkerRequest>) {
        while let Ok(mut worker_request) = queue.recv().await {
            let request = worker_request.request;
            let mut responder = &mut worker_request.responder;
            let result = AssertUnwindSafe(self.handle_request(&mut responder, request))
                .catch_unwind()
                .await;
            match result {
                Err(e) => {
                    responder.try_respond_err(ChromiumOxideError::BrowserError(format!(
                        "Worker {} panicked during request handling: {:?}",
                        self.id, e
                    )));
                }
                Ok(result) => {
                    if let Err(e) = result {
                        responder.try_respond_err(e);
                    }
                }
            }

            if !responder.has_responded() {
                eprintln!("Warning: Worker {} did not respond to request", self.id);
                std::process::exit(1);
            }
        }

        println!("Worker {} shutting down", self.id);
    }

    async fn handle_request(
        &mut self,
        responder: &mut WorkerRequestResponder,
        request: PdfRequest,
    ) -> Result<(), ChromiumOxideError> {
        if let Some(cookies) = request.cookies {
            let mut cookie_params = Vec::new();
            for cookie in cookies {
                let same_site = match cookie.same_site.as_deref() {
                    Some("Lax") => CookieSameSite::Lax,
                    Some("Strict") => CookieSameSite::Strict,
                    Some("None") => CookieSameSite::None,
                    _ => CookieSameSite::Lax,
                };

                let cookie_param = CookieParam::builder()
                    .name(&cookie.name)
                    .value(&cookie.value)
                    .domain(&cookie.domain)
                    .path("/")
                    .secure(false)
                    .http_only(false)
                    .same_site(same_site)
                    .build()
                    .map_err(|e| {
                        ChromiumOxideError::BrowserError(format!("Failed to build cookie: {}", e))
                    })?;
                cookie_params.push(cookie_param);
            }

            if !cookie_params.is_empty() {
                self.browser.set_cookies(cookie_params).await.map_err(|e| {
                    ChromiumOxideError::BrowserError(format!("Failed to set cookies: {}", e))
                })?;
            }
        }
        let page = self.browser.new_page(&request.url).await.map_err(|e| {
            ChromiumOxideError::NavigationError(format!("Failed to open page: {}", e))
        })?;
        // Wait for element specified in request or default to readyForPrint
        let wait_selector = request.wait_for.as_deref().unwrap_or("#readyForPrint");
        if let Err(e) = self.wait_for_element(&page, wait_selector).await {
            page.close()
                .await
                .unwrap_or_else(|e| eprintln!("Failed to close page: {}", e));
            return Err(ChromiumOxideError::BrowserError(format!(
                "Failed to find element '{}': {}",
                wait_selector, e
            )));
        }
        // Generate PDF with options from request
        let mut pdf_params = PrintToPdfParams::builder();
        pdf_params = pdf_params.generate_tagged_pdf(false);

        // Apply PDF options if provided
        if let Some(options) = &request.options {
            if let Some(print_background) = options.print_background {
                pdf_params = pdf_params.print_background(print_background);
            }

            if let Some(display_header_footer) = options.display_header_footer {
                pdf_params = pdf_params.display_header_footer(display_header_footer);
            }

            if let Some(header_template) = &options.header_template {
                pdf_params = pdf_params.header_template(header_template);
            }

            if let Some(footer_template) = &options.footer_template {
                pdf_params = pdf_params.footer_template(footer_template);
            }

            if let Some(format) = &options.format {
                // ChromiumOxide uses paper format enum, map common formats
                match format.as_str() {
                    "A4" => pdf_params = pdf_params.paper_width(8.27).paper_height(11.7),
                    "A3" => pdf_params = pdf_params.paper_width(11.7).paper_height(16.5),
                    "Letter" => pdf_params = pdf_params.paper_width(8.5).paper_height(11.0),
                    _ => {} // Keep default
                }
            }

            if let Some(margin) = &options.margin {
                if let Some(top) = &margin.top {
                    if let Ok(val) = parse_margin(top) {
                        pdf_params = pdf_params.margin_top(val);
                    }
                }
                if let Some(bottom) = &margin.bottom {
                    if let Ok(val) = parse_margin(bottom) {
                        pdf_params = pdf_params.margin_bottom(val);
                    }
                }
                if let Some(left) = &margin.left {
                    if let Ok(val) = parse_margin(left) {
                        pdf_params = pdf_params.margin_left(val);
                    }
                }
                if let Some(right) = &margin.right {
                    if let Ok(val) = parse_margin(right) {
                        pdf_params = pdf_params.margin_right(val);
                    }
                }
            }
        }

        let pdf_result = page.pdf(pdf_params.build()).await;
        let err = match pdf_result {
            Ok(data) => {
                responder.try_respond_ok(data, RequestInfo { url: request.url });
                None
            }
            Err(e) => Some(ChromiumOxideError::PdfGenerationError(format!(
                "Failed to generate PDF: {}",
                e
            ))),
        };

        page.close()
            .await
            .unwrap_or_else(|e| eprintln!("Failed to close page: {}", e));

        // Clear cookies after each request for security
        self.browser.clear_cookies().await.map_err(|e| {
            ChromiumOxideError::BrowserError(format!("Failed to clear cookies: {}", e))
        })?;

        if let Some(err) = err {
            return Err(err);
        }

        Ok(())
    }

    async fn wait_for_element(
        &mut self,
        page: &Page,
        selector: &str,
    ) -> Result<(), ChromiumOxideError> {
        let wait_result = timeout(Duration::from_secs(15), async {
            let js_code = format!(
                r#"
                new Promise((resolve, reject) => {{
                    function checkElement() {{
                        const element = document.querySelector('{}');
                        if (element) {{
                            resolve(true);
                        }} else {{
                            requestAnimationFrame(checkElement);
                        }}
                    }}
                    checkElement();
                }});
            "#,
                selector
            );

            match page.evaluate(js_code.as_str()).await {
                Ok(_) => Ok(()),
                Err(e) => Err(ChromiumOxideError::BrowserError(format!(
                    "JavaScript evaluation failed: {}",
                    e
                ))),
            }
        })
        .await;

        match wait_result {
            Ok(result) => result,
            Err(_) => {
                // Timeout occurred, let's debug
                self.debug_page_state(page).await;
                Err(ChromiumOxideError::TimeoutError(format!(
                    "Timeout waiting for '{}' element",
                    selector
                )))
            }
        }
    }

    async fn debug_page_state(&mut self, page: &Page) {
        // Debug: get HTML content
        match page.content().await {
            Ok(html) => {
                println!("=== HTML CONTENT DEBUG (length: {}) ===", html.len());
                // println!("{}", html);
                println!("=== END HTML CONTENT ===");
            }
            Err(e) => {
                println!("Failed to get HTML content for debugging: {}", e);
            }
        }

        // Debug: get console information
        match page
            .evaluate(
                r#"
            JSON.stringify({
                location: window.location.href,
                readyState: document.readyState,
                title: document.title
            })
        "#,
            )
            .await
        {
            Ok(result) => {
                println!("=== CONSOLE DEBUG ===");
                println!("{:?}", result);
                println!("=== END CONSOLE DEBUG ===");
            }
            Err(e) => {
                println!("Failed to get console info: {}", e);
            }
        }
    }
}

fn create_browser_config(
    chrome_path: &Path,
    user_data_dir: &Path,
) -> Result<BrowserConfig, ChromiumOxideError> {
    std::fs::create_dir_all(user_data_dir).map_err(|e| {
        ChromiumOxideError::BrowserError(format!("Failed to create user data dir: {}", e))
    })?;

    BrowserConfig::builder()
        .headless_mode(HeadlessMode::New)
        .user_data_dir(user_data_dir)
        .chrome_executable(chrome_path)
        .arg("--no-sandbox")
        .arg("--disable-dev-shm-usage")
        .arg("--no-first-run")
        .arg("--enable-logging")
        .arg("--v=1")
        .build()
        .map_err(|e| {
            ChromiumOxideError::BrowserError(format!("Failed to build browser config: {}", e))
        })
}

// Helper function to parse margin strings like "0.75in" to inches as f64
fn parse_margin(margin_str: &str) -> Result<f64, ChromiumOxideError> {
    if margin_str.ends_with("in") {
        margin_str[..margin_str.len() - 2]
            .parse::<f64>()
            .map_err(|_| ChromiumOxideError::BrowserError("Invalid margin format".to_string()))
    } else if margin_str.ends_with("px") {
        // Convert pixels to inches (96 DPI)
        let px = margin_str[..margin_str.len() - 2]
            .parse::<f64>()
            .map_err(|_| ChromiumOxideError::BrowserError("Invalid margin format".to_string()))?;
        Ok(px / 96.0)
    } else {
        // Assume it's already in inches
        margin_str
            .parse::<f64>()
            .map_err(|_| ChromiumOxideError::BrowserError("Invalid margin format".to_string()))
    }
}
