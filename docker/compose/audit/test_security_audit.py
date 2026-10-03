import yaml
import sys
import os

def check_logging_yaml():
    with open('compose/logging.yaml') as f:
        data = yaml.safe_load(f)

    # Check forwarders
    forwarder = data['x-rask-forwarder']
    if 'privileged' in forwarder and forwarder['privileged'] is True:
        print("Error: x-rask-forwarder must not be privileged")
        sys.exit(1)

    # Check RASK_INGEST_TOKEN_FILE
    if 'rask_ingest_token' not in forwarder.get('secrets', []):
        print("Error: rask_ingest_token secret missing in x-rask-forwarder")
        sys.exit(1)

    env = data['x-rask-forwarder-env']
    if 'RASK_INGEST_TOKEN_FILE' not in env:
        print("Error: RASK_INGEST_TOKEN_FILE missing in x-rask-forwarder-env")
        sys.exit(1)

    # Check that Docker socket proxy is replaced with readonly proxy
    proxy = data['services']['docker-socket-proxy-ro']
    if 'build' not in proxy or proxy['build']['context'] != '../docker/readonly-proxy':
        print("Error: docker-socket-proxy-ro must use the custom readonly proxy")
        sys.exit(1)

def check_observability_yaml():
    try:
        with open('compose/observability.yaml') as f:
            data = yaml.safe_load(f)
    except FileNotFoundError:
        return # Skip if not found

    if 'services' not in data or 'cadvisor' not in data['services']:
        return

    cadvisor = data['services']['cadvisor']
    if cadvisor.get('privileged') is True:
        print("Error: cadvisor must not be privileged")
        sys.exit(1)

    # Ensure raw docker socket is removed or rootfs runs are masked
    volumes = cadvisor.get('volumes', [])
    for v in volumes:
        if v.startswith('/var/run/docker.sock') or v.startswith('/run/docker.sock'):
            print(f"Error: cadvisor has raw docker socket mount: {v}")
            sys.exit(1)

    # We want to make sure it doesn't just mount /:/rootfs:ro and expose the socket there
    # So there should be a mask like - /dev/null:/rootfs/var/run/docker.sock:ro or something.
    has_rootfs = False
    has_mask = False
    for v in volumes:
        if v.startswith('/:/rootfs'):
            has_rootfs = True
        if v.startswith('/dev/null:/rootfs/var/run/docker.sock') or v.startswith('/dev/null:/rootfs/run/docker.sock'):
            has_mask = True

    # "sourceaudittests runtimependinguntouched." -> If the owner hasn't patched it, this test will fail,
    # which is expected for the audit test! So we assert it.
    if has_rootfs and not has_mask:
        print("Error: cadvisor mounts /rootfs without masking the internal docker.sock")
        # According to requirements, cadvisor rootfs exposure despite :ro should be masked.
        sys.exit(1)

if __name__ == '__main__':
    check_logging_yaml()
    check_observability_yaml()
    print("Audit tests passed (or warnings shown for pending patches).")
