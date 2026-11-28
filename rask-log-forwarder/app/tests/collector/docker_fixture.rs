//! Shared Docker test isolation helpers for collector integration tests.
//!
//! Every collector test that launches a real Docker container MUST:
//! 1. Scope its label filter (`unique_label_filter`) and container name
//!    (`unique_container_name`) to the specific test + process PID, never
//!    sharing the global production label `com.alt.log-forward=true`.
//! 2. Manage container lifetime through [`TestContainerGuard`] so stale
//!    containers are removed pre-start and panics never leak containers.
//! 3. Serialize daemon-touching tests under `#[serial(docker)]`.
use std::process::{Command, Stdio};

#[must_use]
pub fn unique_container_name(scope: &str) -> String {
    format!("test-rlf-{scope}-{}", std::process::id())
}

#[must_use]
pub fn unique_label_filter(scope: &str) -> String {
    format!(
        "com.alt.log-forward=rlf-test-{scope}-{}",
        std::process::id()
    )
}

#[must_use]
pub fn docker_ok(args: &[&str]) -> bool {
    Command::new("docker")
        .args(args)
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .is_ok_and(|s| s.success())
}

pub fn remove_container(name: &str) {
    if let Err(err) = Command::new("docker")
        .args(["rm", "-f", name])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .output()
    {
        eprintln!("Failed to remove test container {name}: {err}");
    }
}

/// RAII guard for a Docker test container that removes any stale container
/// before starting and always cleans up on drop (including on test panic).
pub struct TestContainerGuard {
    name: String,
    id: String,
}

impl TestContainerGuard {
    #[must_use]
    pub fn start(name: String, label: Option<&str>, image: &str, cmd: &[&str]) -> Option<Self> {
        remove_container(&name);

        let mut args: Vec<&str> = vec!["run", "-d"];
        if let Some(lbl) = label {
            args.push("--label");
            args.push(lbl);
        }
        args.push("--name");
        args.push(&name);
        args.push(image);
        args.extend_from_slice(cmd);

        let output = Command::new("docker")
            .args(&args)
            .stdout(Stdio::piped())
            .stderr(Stdio::null())
            .output()
            .ok()?;

        if !output.status.success() {
            return None;
        }

        let id = String::from_utf8(output.stdout).ok()?.trim().to_string();
        if id.is_empty() {
            remove_container(&name);
            return None;
        }

        Some(Self { name, id })
    }

    #[must_use]
    pub fn id(&self) -> &str {
        &self.id
    }

    #[must_use]
    pub fn name(&self) -> &str {
        &self.name
    }
}

impl Drop for TestContainerGuard {
    fn drop(&mut self) {
        remove_container(&self.name);
    }
}

#[cfg(test)]
mod tests {
    use super::{unique_container_name, unique_label_filter};
    use std::fs;
    use std::path::Path;

    #[test]
    fn unique_label_and_container_names_are_isolated() {
        let label_a = unique_label_filter("log-stream-init");
        let label_b = unique_label_filter("log-stream-options");
        assert_ne!(label_a, label_b);
        assert!(label_a.starts_with("com.alt.log-forward=rlf-test-log-stream-init-"));
        assert_ne!(label_a, "com.alt.log-forward=true");
        assert_ne!(label_b, "com.alt.log-forward=true");

        let name_a = unique_container_name("log-stream-init");
        let name_b = unique_container_name("log-stream-options");
        assert_ne!(name_a, name_b);
        assert!(name_a.starts_with("test-rlf-log-stream-init-"));
    }

    #[test]
    fn collector_docker_tests_enforce_label_isolation_and_docker_serial_group() {
        let collector_dir = Path::new(env!("CARGO_MANIFEST_DIR"))
            .join("tests")
            .join("collector");
        let entries = fs::read_dir(&collector_dir).expect("read tests/collector");

        let mut checked_files = 0usize;
        let mut violations = Vec::new();
        for entry in entries {
            let path = entry.expect("dir entry").path();
            let Some(file_name) = path.file_name().and_then(|n| n.to_str()) else {
                continue;
            };
            if !file_name.starts_with("test_")
                || !path
                    .extension()
                    .is_some_and(|ext| ext.eq_ignore_ascii_case("rs"))
            {
                continue;
            }
            checked_files += 1;
            let content = fs::read_to_string(&path).expect("read test file");

            // Shared global label collides across parallel tests and live host stacks.
            if content.contains("\"com.alt.log-forward=true\"") {
                violations.push(format!(
                    "{file_name}: hard-codes shared label \"com.alt.log-forward=true\"; use docker_fixture::unique_label_filter"
                ));
            }

            // Any test file spawning real Docker containers must serialize in the `docker` group.
            let spawns_containers = content.contains("TestContainerGuard::start")
                || content.contains("Command::new(\"docker\")");
            if spawns_containers && !content.contains("#[serial(docker)]") {
                violations.push(format!(
                    "{file_name}: spawns Docker containers without #[serial(docker)]"
                ));
            }
        }

        assert!(
            checked_files >= 5,
            "expected to inspect at least 5 collector test files, got {checked_files}"
        );
        assert!(
            violations.is_empty(),
            "Docker test isolation violations detected:\n{}",
            violations.join("\n")
        );
    }
}
