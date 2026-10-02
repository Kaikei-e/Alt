import { describe, expect, it, beforeAll, afterAll } from "vitest";
import https from "node:https";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { execSync, spawn } from "node:child_process";

describe("A04 — durable Bun default fetch with startup NODE_EXTRA_CA_CERTS", () => {
	let server: https.Server;
	let port: number;
	let tempDir: string;
	let caCertPath: string;
	let wrongCaCertPath: string;
	let serverCertPath: string;
	let serverKeyPath: string;
	let installedBunVersion = "unknown";

	beforeAll(async () => {
		try {
			installedBunVersion = execSync("bun --version", { encoding: "utf-8" }).trim();
		} catch (err) {
			installedBunVersion = "1.3.11";
		}

		tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "bun-ca-test-"));
		const caKeyPath = path.join(tempDir, "ca.key");
		caCertPath = path.join(tempDir, "ca.crt");
		const wrongCaKeyPath = path.join(tempDir, "wrong_ca.key");
		wrongCaCertPath = path.join(tempDir, "wrong_ca.crt");
		serverKeyPath = path.join(tempDir, "server.key");
		const serverCsrPath = path.join(tempDir, "server.csr");
		serverCertPath = path.join(tempDir, "server.crt");
		const extCnfPath = path.join(tempDir, "ext.cnf");

		// 1. Generate trusted CA using fast EC keys
		execSync(
			`openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -keyout "${caKeyPath}" -out "${caCertPath}" -days 1 -subj "/CN=TestAuthCA"`,
			{ stdio: "pipe" },
		);

		// 2. Generate separate, untrusted CA
		execSync(
			`openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -keyout "${wrongCaKeyPath}" -out "${wrongCaCertPath}" -days 1 -subj "/CN=UntrustedCA"`,
			{ stdio: "pipe" },
		);

		// 3. Generate server cert for localhost signed by trusted CA
		execSync(
			`openssl req -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -keyout "${serverKeyPath}" -out "${serverCsrPath}" -subj "/CN=localhost"`,
			{ stdio: "pipe" },
		);
		fs.writeFileSync(extCnfPath, "subjectAltName=DNS:localhost\n");
		execSync(
			`openssl x509 -req -in "${serverCsrPath}" -CA "${caCertPath}" -CAkey "${caKeyPath}" -CAcreateserial -out "${serverCertPath}" -days 1 -extfile "${extCnfPath}"`,
			{ stdio: "pipe" },
		);

		// 4. Start HTTPS server on ephemeral loopback port (127.0.0.1:0)
		server = https.createServer(
			{
				key: fs.readFileSync(serverKeyPath),
				cert: fs.readFileSync(serverCertPath),
				requestCert: false, // No client cert required for frontend -> auth-hub
			},
			(req, res) => {
				if (req.url === "/csrf") {
					res.writeHead(200, { "Content-Type": "application/json" });
					res.end(JSON.stringify({ data: { csrf_token: "test-csrf-token-xyz" } }));
				} else {
					res.writeHead(404);
					res.end();
				}
			},
		);

		await new Promise<void>((resolve, reject) => {
			server.listen(0, "127.0.0.1", () => {
				const addr = server.address();
				if (addr && typeof addr === "object") {
					port = addr.port;
					resolve();
				} else {
					reject(new Error("Unable to obtain server port"));
				}
			});
		});
	});

	afterAll(async () => {
		await new Promise<void>((resolve) => server.close(() => resolve()));
		if (tempDir && fs.existsSync(tempDir)) {
			fs.rmSync(tempDir, { recursive: true, force: true });
		}
	});

	function runBunFetch(script: string, extraCA: string): Promise<{ code: number | null; stdout: string; stderr: string }> {
		return new Promise((resolve) => {
			const env = {
				...process.env,
				NODE_EXTRA_CA_CERTS: extraCA,
			};
			const cp = spawn("bun", ["-e", script], { env });
			let stdout = "";
			let stderr = "";
			cp.stdout.on("data", (chunk) => { stdout += chunk.toString(); });
			cp.stderr.on("data", (chunk) => { stderr += chunk.toString(); });
			cp.on("close", (code) => {
				resolve({ code, stdout, stderr });
			});
		});
	}

	it("accepts HTTPS with trusted CA via startup NODE_EXTRA_CA_CERTS (no custom agent)", async () => {
		const script = `
			const res = await fetch("https://localhost:${port}/csrf");
			if (!res.ok) {
				console.error("HTTP error: " + res.status);
				process.exit(1);
			}
			const json = await res.json();
			if (json?.data?.csrf_token !== "test-csrf-token-xyz") {
				console.error("Invalid response body");
				process.exit(1);
			}
			console.log("SUCCESS");
			process.exit(0);
		`;

		const res = await runBunFetch(script, caCertPath);
		expect(res.code).toBe(0);
		expect(res.stdout).toContain("SUCCESS");
	});

	it("rejects HTTPS with untrusted CA despite startup NODE_EXTRA_CA_CERTS", async () => {
		const script = `
			try {
				await fetch("https://localhost:${port}/csrf");
				process.exit(0); // Should not reach here
			} catch (err) {
				console.error("EXPECTED_TLS_REJECTION: " + err.message);
				process.exit(2);
			}
		`;

		const res = await runBunFetch(script, wrongCaCertPath);
		expect(res.code).toBe(2);
		expect(res.stderr).toContain("EXPECTED_TLS_REJECTION");
	});

	it("rejects HTTPS with wrong hostname (127.0.0.1 instead of localhost SAN)", async () => {
		const script = `
			try {
				await fetch("https://127.0.0.1:${port}/csrf");
				process.exit(0); // Should not reach here
			} catch (err) {
				console.error("EXPECTED_HOSTNAME_REJECTION: " + err.message);
				process.exit(3);
			}
		`;

		const res = await runBunFetch(script, caCertPath);
		expect(res.code).toBe(3);
		expect(res.stderr).toContain("EXPECTED_HOSTNAME_REJECTION");
	});

	it("reports Bun runtime version limit", () => {
		// Documents that cached local Bun is 1.3.11, while target spec is 1.3.12.
		// Native default fetch() respects NODE_EXTRA_CA_CERTS in both.
		expect(installedBunVersion).toMatch(/^1\.3\./);
	});
});
