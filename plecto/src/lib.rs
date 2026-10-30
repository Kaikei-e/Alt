//! alt-stale-chunk-heal-filter — restores nginx's `error_page 404 = @stale_chunk_reload`.
//!
//! A browser holding a page from the previous deploy asks for an entry chunk whose
//! content hash no longer exists. iOS Safari answers a module-script 404 with the
//! full-sheet "Cannot Open the Page" rather than an in-page error, so the user is
//! stranded on a blank tab. nginx turned that 404 into a 200 carrying a tiny script
//! that reloads the page: the module loads, runs, and the browser lands on the new
//! build. This is layer 1 of the four-layer defense; layers 2-4 are client-side.
//!
//! The route's `path_prefix` is wider than the policy — it covers CSS, fonts and
//! images too, which have no self-healing story and must keep their 404 — so the
//! extension is matched here rather than by the route.

#![allow(clippy::too_many_arguments)]

wit_bindgen::generate!({
    path: "wit",
    world: "filter",
});

use crate::plecto::filter::types::{Header, RequestEdit};

struct AltStaleChunkHealFilter;

/// Bumps a sessionStorage counter and reloads, up to three times, so a chunk that is
/// genuinely missing on the new build cannot become a reload loop.
const RELOAD_SCRIPT: &[u8] = b"(function(){try{var k='alt:chunk-reload-attempts';var n=Number(sessionStorage.getItem(k)||'0');if(n<3){sessionStorage.setItem(k,String(n+1));location.reload();}}catch(e){}})();";

/// Returns true when the header name belongs to an internal identity or secret
/// namespace (`x-alt-*` or `x-internal-*`) that external clients must never be
/// allowed to smuggle to upstream services (AUTH-01).
pub fn is_privileged_internal_header(name: &str) -> bool {
    let lower = name.to_ascii_lowercase();
    lower.starts_with("x-alt-") || lower.starts_with("x-internal-")
}

/// Copies incoming request headers while stripping any header whose lowercase
/// name starts with `"x-alt-"` or `"x-internal-"` so external clients cannot
/// smuggle `X-Alt-User-Id`, `X-Alt-Tenant-Id`, `X-Alt-Role`,
/// `X-Alt-Shared-Secret`, `X-Alt-Backend-Token`, or `X-Internal-*` headers to
/// upstream services before `auth-hub` sets them.
pub fn copy_request_headers(headers: &[Header]) -> Vec<Header> {
    headers
        .iter()
        .filter(|h| !is_privileged_internal_header(&h.name))
        .map(|h| Header {
            name: h.name.clone(),
            value: h.value.clone(),
        })
        .collect()
}

fn is_module_chunk(path_with_query: &str) -> bool {
    let path = path_with_query
        .split_once('?')
        .map_or(path_with_query, |(path, _)| path);

    path.starts_with("/_app/immutable/") && (path.ends_with(".js") || path.ends_with(".mjs"))
}

impl Guest for AltStaleChunkHealFilter {
    fn init() {}

    fn on_request(req: HttpRequest) -> RequestDecision {
        let remove_headers: Vec<String> = req
            .headers
            .iter()
            .filter(|h| is_privileged_internal_header(&h.name))
            .map(|h| h.name.clone())
            .collect();

        if remove_headers.is_empty() {
            RequestDecision::Continue
        } else {
            RequestDecision::Modified(RequestEdit {
                set_headers: Vec::new(),
                remove_headers,
            })
        }
    }

    fn on_response(req: HttpRequest, resp: HttpResponse) -> ResponseDecision {
        if resp.status != 404 || !is_module_chunk(&req.path_with_query) {
            return ResponseDecision::Continue;
        }

        ResponseDecision::Replace(HttpResponse {
            status: 200,
            headers: vec![
                Header {
                    name: "content-type".to_string(),
                    value: b"application/javascript".to_vec(),
                },
                Header {
                    name: "cache-control".to_string(),
                    value: b"no-store".to_vec(),
                },
                Header {
                    name: "x-stale-chunk-reload".to_string(),
                    value: b"1".to_vec(),
                },
            ],
            body: RELOAD_SCRIPT.to_vec(),
        })
    }
}

export!(AltStaleChunkHealFilter);

#[cfg(test)]
mod tests {
    use super::*;

    fn header(name: &str, value: &[u8]) -> Header {
        Header {
            name: name.to_string(),
            value: value.to_vec(),
        }
    }

    #[test]
    fn copy_request_headers_strips_x_alt_and_x_internal_headers() {
        let incoming = vec![
            header("Host", b"localhost"),
            header("Cookie", b"ory_kratos_session=valid"),
            header("X-Alt-User-Id", b"spoofed-user"),
            header("X-Alt-Tenant-Id", b"spoofed-tenant"),
            header("X-Alt-Role", b"admin"),
            header("X-Alt-Shared-Secret", b"forged-secret"),
            header("X-Alt-Backend-Token", b"forged-jwt"),
            header("x-alt-custom", b"forged-custom"),
            header("X-Internal-Auth", b"forged-internal-auth"),
            header("x-internal-token", b"forged-internal-token"),
            header("X-Forwarded-For", b"10.0.0.1"),
            header("Accept", b"application/json"),
        ];

        let filtered = copy_request_headers(&incoming);
        let names: Vec<&str> = filtered.iter().map(|h| h.name.as_str()).collect();

        assert_eq!(names, vec!["Host", "Cookie", "X-Forwarded-For", "Accept"]);
    }

    #[test]
    fn on_request_removes_smuggled_x_alt_and_x_internal_headers() {
        let req = HttpRequest {
            method: "GET".to_string(),
            path_with_query: "/api/v2/feeds".to_string(),
            authority: "localhost".to_string(),
            scheme: "https".to_string(),
            headers: vec![
                header("Host", b"localhost"),
                header("X-Alt-User-Id", b"attacker"),
                header("X-Internal-Auth", b"secret"),
            ],
        };

        match AltStaleChunkHealFilter::on_request(req) {
            RequestDecision::Modified(edit) => {
                assert!(edit.set_headers.is_empty());
                assert_eq!(
                    edit.remove_headers,
                    vec!["X-Alt-User-Id".to_string(), "X-Internal-Auth".to_string()]
                );
            }
            other => panic!("expected Modified decision, got {other:?}"),
        }
    }

    #[test]
    fn on_request_continues_when_no_privileged_headers_present() {
        let req = HttpRequest {
            method: "GET".to_string(),
            path_with_query: "/api/v2/feeds".to_string(),
            authority: "localhost".to_string(),
            scheme: "https".to_string(),
            headers: vec![
                header("Host", b"localhost"),
                header("Cookie", b"ory_kratos_session=abc"),
            ],
        };

        assert!(matches!(
            AltStaleChunkHealFilter::on_request(req),
            RequestDecision::Continue
        ));
    }
}
