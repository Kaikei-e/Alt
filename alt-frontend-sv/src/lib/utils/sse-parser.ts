/**
 * SSE (Server-Sent Events) stream parser
 * Parses a ReadableStream into typed SSE events
 */

/**
 * Generic SSE Event
 */
export interface SSEEvent {
	id?: string;
	event: string;
	data: string;
	retry?: number;
}

/**
 * Parses a readable stream into SSE events
 */
export async function* parseSSEStream(
	reader: ReadableStreamDefaultReader<Uint8Array>,
): AsyncGenerator<SSEEvent> {
	const decoder = new TextDecoder("utf-8");
	let buffer = "";
	let currentEvent: SSEEvent = { event: "message", data: "" };
	let hasData = false;

	try {
		while (true) {
			const { done, value } = await reader.read();
			if (done) {
				// Process last bits if any
				if (buffer.trim()) {
					const lines = buffer.split("\n");
					for (const line of lines) {
						const trimmed = line.trim();
						// Simple logical check for data lines in leftover buffer
						if (trimmed.startsWith("data:")) {
							let content = line.substring(line.indexOf(":") + 1);
							if (content.startsWith(" ")) content = content.substring(1);
							currentEvent.data += content + "\n";
							hasData = true;
						}
					}
					if (hasData) {
						const data = currentEvent.data.endsWith("\n")
							? currentEvent.data.slice(0, -1)
							: currentEvent.data;
						yield { ...currentEvent, data };
					}
				}
				break;
			}

			if (value) {
				const chunk = decoder.decode(value, { stream: true });
				buffer += chunk;

				let boundary = buffer.indexOf("\n");
				while (boundary !== -1) {
					const line = buffer.slice(0, boundary);
					buffer = buffer.slice(boundary + 1);

					const trimmed = line.trim();
					if (!trimmed) {
						// End of event
						if (hasData) {
							const data = currentEvent.data.endsWith("\n")
								? currentEvent.data.slice(0, -1)
								: currentEvent.data;
							yield { ...currentEvent, data };
						}
						currentEvent = { event: "message", data: "" };
						hasData = false;
					} else if (trimmed.startsWith("event:")) {
						currentEvent.event = trimmed.slice(6).trim();
					} else if (trimmed.startsWith("data:")) {
						let content = line.substring(line.indexOf(":") + 1);
						if (content.startsWith(" ")) content = content.substring(1);
						currentEvent.data += content + "\n";
						hasData = true;
					} else if (trimmed.startsWith("id:")) {
						currentEvent.id = trimmed.slice(3).trim();
					} else if (trimmed.startsWith(":")) {
						// Comment
					}

					boundary = buffer.indexOf("\n");
				}
			}
		}
	} finally {
		try {
			reader.releaseLock();
		} catch (e) {}
	}
}
