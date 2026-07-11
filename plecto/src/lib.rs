//! alt-security-headers-filter — replaces nginx/conf.d/snippets/security-headers.conf.
//!
//! Stamps the same four baseline security headers Alt's nginx config applied via
//! `include conf.d/snippets/security-headers.conf` on every location. Plecto has no
//! static per-route response-header config, so every response header change — even
//! this one — needs a WASM filter. Attached to every [[route]] in manifest.toml.
//!
//! Stateless; no capabilities used beyond what the `filter` world lends by default.

#![allow(clippy::too_many_arguments)]

wit_bindgen::generate!({
    path: "wit",
    world: "filter",
});

use crate::plecto::filter::types::{Header, ResponseEdit};

struct AltSecurityHeadersFilter;

impl Guest for AltSecurityHeadersFilter {
    fn init() {}

    fn on_request(_req: HttpRequest) -> RequestDecision {
        RequestDecision::Continue
    }

    fn on_response(_req: HttpRequest, _resp: HttpResponse) -> ResponseDecision {
        ResponseDecision::Modified(ResponseEdit {
            set_status: None,
            set_headers: vec![
                Header {
                    name: "x-frame-options".to_string(),
                    value: b"SAMEORIGIN".to_vec(),
                },
                Header {
                    name: "x-content-type-options".to_string(),
                    value: b"nosniff".to_vec(),
                },
                Header {
                    name: "referrer-policy".to_string(),
                    value: b"strict-origin-when-cross-origin".to_vec(),
                },
                Header {
                    name: "permissions-policy".to_string(),
                    value: b"geolocation=(), camera=(), microphone=()".to_vec(),
                },
            ],
            remove_headers: vec![],
        })
    }
}

export!(AltSecurityHeadersFilter);
