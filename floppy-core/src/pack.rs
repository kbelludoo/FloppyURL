use base64::engine::general_purpose::URL_SAFE_NO_PAD;
use base64::Engine;
use flate2::write::DeflateEncoder;
use flate2::Compression;
use minify_html::{minify, Cfg};
use regex::Regex;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::io::Write;
use std::sync::OnceLock;

static RE_HEAD: OnceLock<Regex> = OnceLock::new();

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PackResponse {
    pub status: String,
    pub engine: String,
    pub algorithm: String,
    pub original_url: String,
    pub original_bytes: usize,
    pub compressed_bytes: usize,
    pub encoded_chars: usize,
    pub reduction_pct: f64,
    pub sha256: String,
    pub boot_url: String,
}

/// Injects base tag and parent navigation listener into HTML.
pub fn inject_pocketweb_script(html: &str, target_url: &str) -> String {
    let injection = format!(
        r#"<base href="{target_url}"><script>document.addEventListener('click',function(e){{var a=e.target.closest('a');if(a&&a.href&&!a.href.startsWith('javascript:')&&!a.href.startsWith('#')){{e.preventDefault();window.parent.postMessage({{type:'POCKET_NAV',url:a.href}},'*');}}}});</script>"#
    );

    let re_head = RE_HEAD.get_or_init(|| Regex::new(r"(?i)<head>").unwrap());

    if re_head.is_match(html) {
        re_head.replace(html, format!("<head>{injection}")).to_string()
    } else {
        format!("{injection}{html}")
    }
}

/// Minifies, compresses with Deflate (BestCompression), and generates the FloppyURL bootloader URL.
pub fn pack_html(raw_html: &str, target_url: &str) -> Result<PackResponse, String> {
    let original_bytes = raw_html.len();

    // 1. Injetar navegação do PocketWeb
    let injected = inject_pocketweb_script(raw_html, target_url);

    // 2. Minificação ultra-rápida em Rust com minify-html
    let mut cfg = Cfg::new();
    cfg.minify_css = true;
    cfg.minify_js = true;
    cfg.keep_comments = false;
    cfg.do_not_minify_doctype = false;

    let minified = minify(injected.as_bytes(), &cfg);

    // 3. Compressão Deflate (Nível 9 - Best Compression)
    let mut encoder = DeflateEncoder::new(Vec::new(), Compression::best());
    encoder
        .write_all(&minified)
        .map_err(|e| format!("Falha na compressão Deflate: {e}"))?;
    let compressed_bytes = encoder
        .finish()
        .map_err(|e| format!("Falha ao finalizar compressão: {e}"))?;

    let comp_size = compressed_bytes.len();

    // 4. Base64 Raw URL Encoding (sem padding '=')
    let encoded = URL_SAFE_NO_PAD.encode(&compressed_bytes);
    let encoded_chars = encoded.len();

    // 5. SHA-256 do payload codificado
    let mut hasher = Sha256::new();
    hasher.update(encoded.as_bytes());
    let chunk_sha = hex::encode(hasher.finalize());

    // 6. Redução percentual
    let reduction_pct = if original_bytes > 0 && original_bytes >= comp_size {
        ((original_bytes - comp_size) as f64 / original_bytes as f64) * 100.0
    } else {
        0.0
    };

    let algo = "deflate";
    let boot_url = format!(
        "https://kbelludoo.github.io/FloppyURL/#v2;{algo};[1/1];{chunk_sha};{chunk_sha};{encoded}"
    );

    Ok(PackResponse {
        status: "success".to_string(),
        engine: "Rust 1.85+ (FloppyCore Engine)".to_string(),
        algorithm: algo.to_string(),
        original_url: target_url.to_string(),
        original_bytes,
        compressed_bytes: comp_size,
        encoded_chars,
        reduction_pct,
        sha256: chunk_sha,
        boot_url,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_pack_html() {
        let sample_html = r#"
        <!DOCTYPE html>
        <html>
            <head>
                <title>Test Page</title>
                <style>
                    body { background-color: #ffffff; color: #000000; }
                </style>
            </head>
            <body>
                <h1>Hello World</h1>
                <p>This is a test of PocketWeb packaging.</p>
                <a href="https://example.com/other">Click here</a>
            </body>
        </html>
        "#;

        let res = pack_html(sample_html, "https://example.com").expect("Pack deve ter sucesso");
        assert_eq!(res.status, "success");
        assert!(res.reduction_pct > 0.0);
        assert!(res.boot_url.starts_with("https://kbelludoo.github.io/FloppyURL/#v2;deflate;[1/1];"));
        assert_eq!(res.sha256.len(), 64);
    }
}
