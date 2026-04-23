/**
 * UUIDv7 generator (RFC 9562 draft / Crockford).
 *
 * Layout (128 bits):
 *   48 bits  unix_ts_ms           — big-endian millisecond timestamp
 *    4 bits  version (= 0111₂)    — marks as v7
 *   12 bits  rand_a               — entropy (monotonic tiebreak within same ms)
 *    2 bits  variant (= 10₂)      — RFC 4122
 *   62 bits  rand_b               — entropy
 *
 * Monotonicity: within the same millisecond the generator feeds back the
 * last 74 random bits + 1 so successive calls are non-decreasing. This
 * matches the "method 1" monotonic strategy in the draft.
 */

let lastTsMs = 0;
let lastRandHi = 0; // 12 bits: rand_a (shared slot with version)
let lastRandLo = 0n; // 62 bits: rand_b (shared slot with variant)

function randBytes(n: number): Uint8Array {
	const buf = new Uint8Array(n);
	crypto.getRandomValues(buf);
	return buf;
}

function freshEntropy(): { hi: number; lo: bigint } {
	const b = randBytes(10);
	// hi: 12 random bits (version nibble overwritten later)
	const hi = ((b[0] << 8) | b[1]) & 0x0fff;
	// lo: 62 random bits (variant bits overwritten later)
	const lo =
		((BigInt(b[2]) << 56n) |
			(BigInt(b[3]) << 48n) |
			(BigInt(b[4]) << 40n) |
			(BigInt(b[5]) << 32n) |
			(BigInt(b[6]) << 24n) |
			(BigInt(b[7]) << 16n) |
			(BigInt(b[8]) << 8n) |
			BigInt(b[9])) &
		0x3fffffffffffffffn;
	return { hi, lo };
}

function hex(value: number | bigint, width: number): string {
	return value.toString(16).padStart(width, "0");
}

export function uuidv7(): string {
	const now = Date.now();
	let hi: number;
	let lo: bigint;

	if (now > lastTsMs) {
		const e = freshEntropy();
		hi = e.hi;
		lo = e.lo;
		lastTsMs = now;
	} else {
		// same (or clock-skewed earlier) ms: bump the 74-bit random counter by 1
		// so the generated id strictly follows the previous one.
		let nextLo = lastRandLo + 1n;
		let nextHi = lastRandHi;
		if (nextLo > 0x3fffffffffffffffn) {
			nextLo = 0n;
			nextHi = (nextHi + 1) & 0x0fff;
		}
		hi = nextHi;
		lo = nextLo;
	}

	lastRandHi = hi;
	lastRandLo = lo;

	const tsHex = hex(lastTsMs, 12); // 48 bits
	const versionHi = 0x7000 | hi; // set version nibble to 7
	const versionHiHex = hex(versionHi, 4);
	const variantLo = (lo | 0x8000000000000000n) & 0xbfffffffffffffffn; // 10xx_xxxx
	const variantLoHex = hex(variantLo, 16);

	return [
		tsHex.slice(0, 8),
		tsHex.slice(8, 12),
		versionHiHex,
		variantLoHex.slice(0, 4),
		variantLoHex.slice(4, 16),
	].join("-");
}
