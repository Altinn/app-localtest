use async_trait::async_trait;
use serde::Deserialize;

pub const MAX_CONCURRENCY: usize = 4;

#[derive(Deserialize, Debug, Clone)]
pub struct PdfRequest {
    pub url: String,
    pub options: Option<PdfOptions>,
    #[serde(rename = "setJavaScriptEnabled")]
    pub set_java_script_enabled: Option<bool>,
    #[serde(rename = "waitFor")]
    pub wait_for: Option<String>,
    pub cookies: Option<Vec<Cookie>>,
}

#[derive(Deserialize, Debug, Clone)]
pub struct PdfOptions {
    #[serde(rename = "headerTemplate")]
    pub header_template: Option<String>,
    #[serde(rename = "footerTemplate")]
    pub footer_template: Option<String>,
    #[serde(rename = "displayHeaderFooter")]
    pub display_header_footer: Option<bool>,
    #[serde(rename = "printBackground")]
    pub print_background: Option<bool>,
    pub format: Option<String>,
    pub margin: Option<PdfMargin>,
}

#[derive(Deserialize, Debug, Clone)]
pub struct PdfMargin {
    pub top: Option<String>,
    pub right: Option<String>,
    pub bottom: Option<String>,
    pub left: Option<String>,
}

#[derive(Deserialize, Debug, Clone)]
pub struct Cookie {
    pub name: String,
    pub value: String,
    pub domain: String,
    #[serde(rename = "sameSite")]
    pub same_site: Option<String>,
}

#[derive(Debug, Clone)]
pub struct RequestInfo {
    pub url: String,
}

#[derive(Debug, Clone)]
pub struct PdfResult {
    pub data: Vec<u8>,
    pub browser: BrowserVersion,
    pub request_info: RequestInfo,
}

#[derive(Debug, Clone)]
pub struct BrowserVersion {
    pub product: &'static str,
    pub protocol_version: &'static str,
    pub revision: &'static str,
    pub user_agent: &'static str,
    pub js_version: &'static str,
}

#[async_trait]
pub trait PdfGenerator {
    async fn generate(
        &self,
        request: PdfRequest,
    ) -> Result<PdfResult, Box<dyn std::error::Error + Send + Sync>>;
}
