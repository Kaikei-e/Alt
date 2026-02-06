// Build-time regex pattern validation
use regex::Regex;
use std::fmt::Write as FmtWrite;
use std::fs::File;
use std::io::Write;

fn main() {
    println!("cargo:rerun-if-changed=build.rs");
    println!("cargo:rerun-if-changed=Cargo.toml");

    // All regex patterns used in the codebase
    let patterns = &[
        // From universal.rs
        (
            r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z?\s+",
            "docker_native_timestamp",
        ),
        (
            r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}",
            "iso_timestamp_fallback",
        ),
        // From services.rs
        (
            r#"^(\S+) \S+ \S+ \[([^\]]+)\] "([A-Z]+) ([^"]*) HTTP/[^"]*" (\d+) (\d+|-)(?: "([^"]*)" "([^"]*)")?.*$"#,
            "nginx_access_full",
        ),
        (
            r#"^(\S+) .+ "([A-Z]+) ([^"]*) HTTP/[^"]*" (\d+) (\d+|-)"#,
            "nginx_access_fallback",
        ),
        (
            r"^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) \[(\w+)\] \d+#\d+: (.+)",
            "nginx_error_full",
        ),
        (
            r"^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) \[(\w+)\] (.+)",
            "nginx_error_fallback",
        ),
        (
            r"^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) .+ (\w+):\s+(.+)",
            "postgres_log",
        ),
        // From simd.rs
        (
            r#"^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) ([^"]*) HTTP/[^"]*" (\d+) (\d+)"#,
            "simd_nginx_access",
        ),
        (
            r#"^(\S+) .+ "(\S+) ([^"]*)" (\d+) (\d+)"#,
            "simd_nginx_access_fallback",
        ),
        (
            r#"^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) ([^"]*) HTTP/[^"]*" (\d+) (\d+) "([^"]*)" "([^"]*)""#,
            "simd_nginx_combined",
        ),
        (
            r#"^(\S+) .+ "(\S+) ([^"]*)" (\d+) (\d+) "([^"]*)" "([^"]*)""#,
            "simd_nginx_combined_fallback",
        ),
        (
            r"^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) \[(\w+)\] (\d+)#(\d+): (.*?)(?:\n)?$",
            "simd_nginx_error",
        ),
        (
            r"^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) \[(\w+)\] (.*)$",
            "simd_nginx_error_fallback",
        ),
    ];

    println!("cargo:info=Starting regex pattern validation...");

    let mut valid_patterns = Vec::new();
    let mut invalid_patterns = Vec::new();

    for &(pattern, name) in patterns {
        match Regex::new(pattern) {
            Ok(_) => {
                valid_patterns.push((pattern, name));
                println!("cargo:warning=✓ Regex '{name}' is valid");
            }
            Err(e) => {
                println!("cargo:warning=❌ Invalid regex pattern '{name}': {e}");
                invalid_patterns.push((pattern, name, e));
            }
        }
    }

    if !invalid_patterns.is_empty() {
        let mut error_msg = String::from("❌ Build failed due to invalid regex patterns:\n");
        for (pattern, name, error) in &invalid_patterns {
            writeln!(error_msg, "  - '{name}': {error} (pattern: {pattern})").unwrap();
        }
        panic!("{}", error_msg);
    }

    // Generate validated regex patterns file
    if let Err(e) = generate_validated_regexes(&valid_patterns) {
        panic!("❌ Failed to generate regex patterns: {e}");
    }

    println!(
        "cargo:info=✓ All {} regex patterns validated successfully",
        valid_patterns.len()
    );
}

fn generate_validated_regexes(patterns: &[(&str, &str)]) -> Result<(), Box<dyn std::error::Error>> {
    let out_dir = std::env::var("OUT_DIR")?;
    let dest_path = std::path::Path::new(&out_dir).join("validated_regexes.rs");
    let mut file = File::create(dest_path)?;

    writeln!(
        file,
        "// Auto-generated regex patterns (validated by build.rs)"
    )?;
    writeln!(file, "use crate::parser::regex_patterns::StaticRegexSet;")?;
    writeln!(file)?;
    writeln!(
        file,
        "/// All validated regex patterns used in the codebase"
    )?;
    writeln!(
        file,
        "pub static VALIDATED_PATTERNS: StaticRegexSet = StaticRegexSet::new(&["
    )?;

    for (pattern, name) in patterns {
        // Use raw string hashes only when pattern contains double quotes
        if pattern.contains('"') {
            writeln!(file, "    (r#\"{pattern}\"#, \"{name}\"),")?;
        } else {
            writeln!(file, "    (r\"{pattern}\", \"{name}\"),")?;
        }
    }

    writeln!(file, "]);")?;
    writeln!(file)?;

    // Generate pattern index constants
    writeln!(file, "/// Pattern indices for type-safe access")?;
    writeln!(file, "pub mod pattern_index {{")?;
    for (i, (_, name)) in patterns.iter().enumerate() {
        let const_name = name.to_uppercase();
        writeln!(file, "    pub const {const_name}: usize = {i};")?;
    }
    writeln!(file, "}}")?;

    writeln!(file)?;
    writeln!(file, "/// Pattern name lookup")?;
    writeln!(
        file,
        "pub fn get_pattern_name(index: usize) -> Option<&'static str> {{"
    )?;
    writeln!(file, "    match index {{")?;
    for (i, (_, name)) in patterns.iter().enumerate() {
        writeln!(file, "        {i} => Some(\"{name}\"),")?;
    }
    writeln!(file, "        _ => None,")?;
    writeln!(file, "    }}")?;
    writeln!(file, "}}")?;

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_all_patterns_compile() {
        let patterns = &[
            (r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}", "iso_timestamp"),
            (
                r#"^(\S+) .+ "([A-Z]+) ([^"]*) HTTP/[^"]*" (\d+) (\d+|-)"#,
                "nginx_access",
            ),
        ];

        for (pattern, name) in patterns {
            let result = Regex::new(pattern);
            assert!(
                result.is_ok(),
                "Pattern '{}' should compile: {:?}",
                name,
                result.err()
            );
        }
    }
}
