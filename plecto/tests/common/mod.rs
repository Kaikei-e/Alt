use toml::Table;

pub fn manifest() -> Table {
    let path = concat!(env!("CARGO_MANIFEST_DIR"), "/manifest.toml");
    std::fs::read_to_string(path)
        .expect("manifest.toml is readable")
        .parse()
        .expect("manifest.toml is valid TOML")
}

pub fn routes(manifest: &Table) -> Vec<&Table> {
    manifest["route"]
        .as_array()
        .expect("manifest declares [[route]] entries")
        .iter()
        .map(|r| r.as_table().expect("route is a table"))
        .collect()
}

pub fn path_prefix(route: &Table) -> &str {
    route["match"]["path_prefix"]
        .as_str()
        .expect("route has match.path_prefix")
}
