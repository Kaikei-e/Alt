// Export README screenshots to WebP for smaller README payload and faster page loads.
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

const INPUT_DIR = path.resolve("test-results/readme-capture");
const OUTPUT_DIR = path.resolve("../docs/images/readme");
const HERO_ONLY_INPUTS = new Set(["home-desktop.png"]);

/**
 * @param {string} filename
 * @returns {number | null}
 */
function targetWidthFor(filename) {
	if (HERO_ONLY_INPUTS.has(filename)) return null;
	if (filename === "hero.png") return 2400;
	if (filename.endsWith("-desktop.png")) return 1920;
	if (filename.endsWith("-phone.png")) return 720;
	return null;
}

if (!fs.existsSync(INPUT_DIR)) {
	console.error(`Input directory not found: ${INPUT_DIR}`);
	process.exit(1);
}

fs.mkdirSync(OUTPUT_DIR, { recursive: true });

const pngFiles = fs
	.readdirSync(INPUT_DIR)
	.filter((name) => name.endsWith(".png"))
	.sort();

for (const filename of pngFiles) {
	const width = targetWidthFor(filename);
	if (width === null) continue;

	const inputPath = path.join(INPUT_DIR, filename);
	const outName = filename.replace(/\.png$/, ".webp");
	const outputPath = path.join(OUTPUT_DIR, outName);

	try {
		execFileSync(
			"cwebp",
			[
				"-quiet",
				"-q",
				"82",
				"-m",
				"6",
				"-sharp_yuv",
				"-resize",
				String(width),
				"0",
				inputPath,
				"-o",
				outputPath,
			],
			{ stdio: "pipe" },
		);
	} catch (err) {
		const code =
			err && typeof err === "object" && "code" in err ? err.code : undefined;
		if (code === "ENOENT") {
			console.error(
				"cwebp binary not found on PATH; install webp tools to export README images.",
			);
		} else {
			const stderr =
				err && typeof err === "object" && "stderr" in err
					? String(err.stderr)
					: String(err);
			console.error(`cwebp failed for ${filename}: ${stderr}`);
		}
		process.exit(1);
	}

	const { size } = fs.statSync(outputPath);
	console.log(`${outName}: ${size} bytes`);
}
