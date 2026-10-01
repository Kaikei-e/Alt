/**
 * Connect-RPC streaming framing helpers for application/connect+json.
 *
 * Each message in a Connect stream consists of a 5-byte envelope prefix:
 * - 1 byte flags: 0x00 for data frame, 0x02 for end-of-stream frame
 * - 4 bytes uint32 big-endian: length of the message payload
 * Followed by the message bytes (JSON encoded in UTF-8).
 */

export type ConnectFrame<T = any> = {
	readonly flag: number;
	readonly data: T;
};

export function encodeConnectFrame(payload: unknown): Buffer {
	const jsonBuf = Buffer.from(JSON.stringify(payload), "utf-8");
	const header = Buffer.alloc(5);
	header.writeUInt8(0x00, 0); // data frame
	header.writeUInt32BE(jsonBuf.length, 1);
	return Buffer.concat([header, jsonBuf]);
}

export function decodeConnectFrames(buf: Buffer): ConnectFrame[] {
	const frames: ConnectFrame[] = [];
	let offset = 0;
	while (offset + 5 <= buf.length) {
		const flag = buf.readUInt8(offset);
		const length = buf.readUInt32BE(offset + 1);
		offset += 5;
		if (offset + length > buf.length) {
			throw new Error(
				`Truncated frame at offset ${offset}: expected ${length} bytes, remaining ${buf.length - offset}`,
			);
		}
		const payload = buf.subarray(offset, offset + length);
		offset += length;
		const json = JSON.parse(payload.toString("utf-8"));
		frames.push({ flag, data: json });
	}
	return frames;
}
