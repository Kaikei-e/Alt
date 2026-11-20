const FRAME_BASE_CSS = `* { box-sizing: border-box; }
.browser-window {
	display: inline-block;
	border-radius: 12px;
	border: 1px solid rgba(40, 35, 28, 0.16);
	overflow: hidden;
	background: #faf9f7;
}
.title-bar {
	position: relative;
	height: 40px;
	background: linear-gradient(180deg, #efedea 0%, #e7e4de 100%);
	border-bottom: 1px solid #d6d2cb;
	display: flex;
	align-items: center;
	padding: 0 16px;
}
.dots {
	display: flex;
	align-items: center;
	gap: 8px;
}
.dot {
	width: 11px;
	height: 11px;
	border-radius: 50%;
	background: #cfcac1;
}
.address-pill {
	position: absolute;
	left: 50%;
	transform: translateX(-50%);
	background: #faf9f7;
	border: 1px solid #d6d2cb;
	border-radius: 7px;
	font-family: "IBM Plex Mono", monospace;
	font-size: 12px;
	line-height: 1.4;
	color: #5a564f;
	padding: 3px 28px;
	letter-spacing: 0.01em;
	white-space: nowrap;
}
.browser-body img {
	display: block;
	height: auto;
}
.phone-frame {
	display: inline-block;
	background: #26231f;
}
.phone-frame img {
	display: block;
	height: auto;
}`;

function escapeHtml(value: string): string {
	return value
		.replaceAll("&", "&amp;")
		.replaceAll("<", "&lt;")
		.replaceAll(">", "&gt;")
		.replaceAll('"', "&quot;");
}

export function browserFrame(options: {
	src: string;
	address: string;
	width: number;
	className?: string;
}): string {
	const classes = options.className
		? `browser-window ${options.className}`
		: "browser-window";
	return `<div class="${classes}">
  <div class="title-bar">
    <div class="dots">
      <span class="dot"></span>
      <span class="dot"></span>
      <span class="dot"></span>
    </div>
    <div class="address-pill">${escapeHtml(options.address)}</div>
  </div>
  <div class="browser-body">
    <img src="${options.src}" alt="" style="width: ${options.width}px;" />
  </div>
</div>`;
}

export function phoneFrame(options: {
	src: string;
	screenWidth: number;
	className?: string;
}): string {
	const padding = Math.round(options.screenWidth * 0.03);
	const screenRadius = Math.round(options.screenWidth * 0.14);
	const outerRadius = screenRadius + padding;
	const classes = options.className
		? `phone-frame ${options.className}`
		: "phone-frame";
	return `<div class="${classes}" style="padding: ${padding}px; border-radius: ${outerRadius}px;">
  <img src="${options.src}" alt="" style="width: ${options.screenWidth}px; border-radius: ${screenRadius}px;" />
</div>`;
}

export function frameDocument(options: {
	body: string;
	extraCss?: string;
}): string {
	const css = options.extraCss
		? `${FRAME_BASE_CSS}\n${options.extraCss}`
		: FRAME_BASE_CSS;
	return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <link rel="preconnect" href="https://fonts.googleapis.com" />
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
    <link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500;600&family=Playfair+Display:ital,wght@0,400;0,600;0,700;0,800;1,400&family=Source+Sans+3:wght@300;400;500;600;700&display=swap" rel="stylesheet" />
    <style>
${css}
    </style>
  </head>
  <body>
    ${options.body}
  </body>
</html>`;
}
