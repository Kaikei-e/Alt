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

use crate::plecto::filter::types::Header;

struct AltStaleChunkHealFilter;

/// Bumps a sessionStorage counter and reloads, up to three times, so a chunk that is
/// genuinely missing on the new build cannot become a reload loop.
const RELOAD_SCRIPT: &[u8] = b"(function(){try{var k='alt:chunk-reload-attempts';var n=Number(sessionStorage.getItem(k)||'0');if(n<3){sessionStorage.setItem(k,String(n+1));location.reload();}}catch(e){}})();";

fn is_module_chunk(path_with_query: &str) -> bool {
    let path = path_with_query
        .split_once('?')
        .map_or(path_with_query, |(path, _)| path);

    path.starts_with("/_app/immutable/") && (path.ends_with(".js") || path.ends_with(".mjs"))
}

impl Guest for AltStaleChunkHealFilter {
    fn init() {}

    fn on_request(_req: HttpRequest) -> RequestDecision {
        RequestDecision::Continue
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
