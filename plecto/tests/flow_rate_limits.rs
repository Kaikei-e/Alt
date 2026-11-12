mod common;

const FLOWS: [&str; 4] = ["login", "registration", "recovery", "verification"];
const MAX_SUSTAINED_PER_SECOND: i64 = 1;
const MAX_BURST: i64 = 10;

#[test]
fn identity_flow_routes_have_a_strict_per_client_limit() {
    let manifest = common::manifest();
    let routes = common::routes(&manifest);

    for flow in FLOWS {
        let prefix = format!("/ory/self-service/{flow}");
        let route = routes
            .iter()
            .find(|r| common::path_prefix(r) == prefix)
            .unwrap_or_else(|| panic!("no dedicated route for {prefix}"));

        assert_eq!(route["upstream"].as_str(), Some("kratos"), "{prefix}");
        assert_eq!(route["strip_prefix"].as_str(), Some("/ory"), "{prefix}");

        let limit = route
            .get("rate_limit")
            .and_then(|l| l.as_table())
            .unwrap_or_else(|| panic!("{prefix} has no rate_limit"));
        assert_eq!(limit["key"].as_str(), Some("client-ip"), "{prefix}");
        assert!(
            limit["rate"].as_integer().unwrap() <= MAX_SUSTAINED_PER_SECOND,
            "{prefix} sustained rate"
        );
        assert!(
            limit["burst"].as_integer().unwrap() <= MAX_BURST,
            "{prefix} burst"
        );
    }
}

#[test]
fn identity_flow_limits_are_stricter_than_the_general_kratos_route() {
    let manifest = common::manifest();
    let routes = common::routes(&manifest);

    let general = routes
        .iter()
        .find(|r| common::path_prefix(r) == "/ory/")
        .expect("general /ory/ route");
    let general_rate = general["rate_limit"]["rate"].as_integer().unwrap();

    for flow in FLOWS {
        let prefix = format!("/ory/self-service/{flow}");
        let route = routes
            .iter()
            .find(|r| common::path_prefix(r) == prefix)
            .unwrap_or_else(|| panic!("no dedicated route for {prefix}"));
        assert!(route["rate_limit"]["rate"].as_integer().unwrap() < general_rate);
    }
}
