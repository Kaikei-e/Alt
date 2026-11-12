mod common;

const HEADER_STRIP_FILTER: &str = "stale-chunk-heal";

#[test]
fn every_forwarding_route_runs_the_header_strip_filter() {
    let manifest = common::manifest();
    let mut missing = Vec::new();

    for route in common::routes(&manifest) {
        let forwards = route.contains_key("upstream") || route.contains_key("backends");
        if !forwards {
            continue;
        }
        let has_filter = route
            .get("filters")
            .and_then(|f| f.as_array())
            .is_some_and(|f| f.iter().any(|id| id.as_str() == Some(HEADER_STRIP_FILTER)));
        if !has_filter {
            missing.push(common::path_prefix(route).to_string());
        }
    }

    assert!(
        missing.is_empty(),
        "routes forwarding to an upstream without the `{HEADER_STRIP_FILTER}` filter: {missing:?}"
    );
}

#[test]
fn the_header_strip_filter_is_declared() {
    let manifest = common::manifest();
    let declared = manifest["filter"]
        .as_array()
        .expect("manifest declares [[filter]] entries")
        .iter()
        .any(|f| f["id"].as_str() == Some(HEADER_STRIP_FILTER));
    assert!(declared);
}
