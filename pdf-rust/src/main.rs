#![allow(dead_code)]

use std::convert::Infallible;
use std::net::SocketAddr;
use std::sync::Arc;

use http_body_util::{BodyExt, Full};
use hyper::body::Bytes;
use hyper::server::conn::http1;
use hyper::service::service_fn;
use hyper::{Request, Response};
use hyper_util::rt::TokioIo;
use serde::Serialize;
use tokio::net::TcpListener;
use tokio::signal;
use tokio::sync::oneshot;

mod internal;
mod pdf;

use crate::internal::{ChromiumOxide, fetcher};
use crate::pdf::{PdfGenerator, PdfRequest};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    println!("Starting PDF service...");
    let output = "./output/";
    let chrome_path = fetcher::fetch(output).await;

    let pdf_service: Arc<dyn PdfGenerator + Send + Sync> = {
        println!("Initializing chromiumoxide PDF service");
        Arc::new(ChromiumOxide::new(&chrome_path).await?)
    };

    println!("PDF service initialized with thread pool");

    let addr = SocketAddr::from(([0, 0, 0, 0], 5010));
    let listener = TcpListener::bind(addr).await?;
    println!("Listening on http://{}", addr);

    let (shutdown_tx, mut shutdown_rx) = oneshot::channel();

    // Spawn a task to listen for shutdown signals
    tokio::spawn(async move {
        shutdown_signal().await;
        println!("Shutdown signal received, initiating graceful shutdown...");
        let _ = shutdown_tx.send(());
    });

    // We start a loop to continuously accept incoming connections
    loop {
        tokio::select! {
            // Wait for shutdown signal
            _ = &mut shutdown_rx => {
                println!("Shutting down server...");
                break;
            }
            // Accept new connections
            result = listener.accept() => {
                match result {
                    Ok((stream, _)) => {
                        let io = TokioIo::new(stream);
                        let pdf_service_clone = Arc::clone(&pdf_service);
                        tokio::task::spawn(async move {
                            if let Err(err) = http1::Builder::new()
                                .serve_connection(
                                    io,
                                    service_fn(move |req| serve(req, Arc::clone(&pdf_service_clone))),
                                )
                                .await
                            {
                                eprintln!("Error serving connection: {:?}", err);
                            }
                        });
                    }
                    Err(e) => {
                        eprintln!("Error accepting connection: {:?}", e);
                        break;
                    }
                }
            }
        }
    }

    println!("PDF service shutdown complete");
    Ok(())
}

async fn shutdown_signal() {
    let ctrl_c = async {
        signal::ctrl_c()
            .await
            .expect("failed to install Ctrl+C handler");
    };

    #[cfg(unix)]
    let terminate = async {
        signal::unix::signal(signal::unix::SignalKind::terminate())
            .expect("failed to install signal handler")
            .recv()
            .await;
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {},
        _ = terminate => {},
    }
}

async fn serve(
    mut req: Request<hyper::body::Incoming>,
    pdf_service: Arc<dyn PdfGenerator + Send + Sync>,
) -> Result<Response<Full<Bytes>>, Infallible> {
    match req.uri().path() {
        "/health" => return Ok(Response::new(Full::new(Bytes::from("OK")))),
        "/pdf" => return pdf(&mut req, pdf_service).await,
        _ => {
            return Ok(Response::builder()
                .status(404)
                .body(Full::new(Bytes::from("Not found")))
                .unwrap());
        }
    }
}

async fn pdf(
    req: &mut Request<hyper::body::Incoming>,
    pdf_service: Arc<dyn PdfGenerator + Send + Sync>,
) -> Result<Response<Full<Bytes>>, Infallible> {
    let content_len = req.headers().get("Content-Length");
    if content_len.is_none() {
        return Ok(create_problem_response(
            "about:blank",
            "Content-Length header missing",
            400,
        ));
    }
    if content_len
        .unwrap()
        .to_str()
        .unwrap()
        .parse::<usize>()
        .unwrap()
        > 10 * 1024
    {
        return Ok(create_problem_response(
            "about:blank",
            "Request body too large",
            400,
        ));
    }

    // Parse the request body to get the URL
    let body = req.body_mut();
    let body_bytes = match body.collect().await {
        Ok(collected) => collected.to_bytes(),
        Err(_) => {
            return Ok(create_problem_response(
                "about:blank",
                "Failed to read request body",
                400,
            ));
        }
    };

    let body_str = match std::str::from_utf8(&body_bytes) {
        Ok(s) => s,
        Err(_) => {
            return Ok(create_problem_response(
                "about:blank",
                "Invalid UTF-8 in request body",
                400,
            ));
        }
    };

    // Parse JSON to get PDF request
    let pdf_request = match serde_json::from_str::<PdfRequest>(body_str) {
        Ok(request) => request,
        Err(e) => {
            return Ok(create_problem_response(
                "about:blank",
                &format!("Invalid JSON in request body: {}", e),
                400,
            ));
        }
    };

    // Generate PDF using the service
    match pdf_service.generate(pdf_request).await {
        Ok(result) => {
            println!(
                "Successfully generated PDF with {} bytes for URL: {}",
                result.data.len(),
                result.request_info.url
            );
            Ok(Response::builder()
                .status(200)
                .header("Content-Type", "application/pdf")
                .header("Content-Length", result.data.len())
                .header("X-Browser-Product", result.browser.product)
                .header("X-Browser-Revision", result.browser.revision)
                .header("X-Browser-ProtocolVersion", result.browser.protocol_version)
                .body(Full::new(Bytes::from(result.data)))
                .unwrap())
        }
        Err(e) => {
            eprintln!("Failed to generate PDF: {}", e);
            Ok(create_problem_response(
                "about:blank",
                &format!("PDF generation failed: {}", e),
                500,
            ))
        }
    }
}

#[derive(Serialize)]
struct ProblemDetails {
    #[serde(rename = "type")]
    problem_type: String,
    title: String,
    status: u16,
}

fn create_problem_response(problem_type: &str, title: &str, status: u16) -> Response<Full<Bytes>> {
    let problem_details = ProblemDetails {
        problem_type: problem_type.to_string(),
        title: title.to_string(),
        status,
    };

    let json = serde_json::to_string(&problem_details).unwrap();

    Response::builder()
        .status(status)
        .header("Content-Type", "application/problem+json")
        .body(Full::new(Bytes::from(json)))
        .unwrap()
}
