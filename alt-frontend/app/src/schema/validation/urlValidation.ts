import * as v from "valibot";

export const safeUrlSchema = v.pipe(
  v.string("Invalid or unsafe URL"),
  v.check(isValidAndSafeUrl, "Invalid or unsafe URL"),
);

// セキュリティテスト用にexport
export const validateUrl = isValidAndSafeUrl;

function isValidAndSafeUrl(url: string): boolean {
  if (!url || typeof url !== "string") {
    return false;
  }

  // Trim whitespace
  url = url.trim();

  if (!url) {
    return false;
  }

  try {
    const parsedUrl = new URL(url);
    
    // Only allow HTTP and HTTPS protocols
    if (!["http:", "https:"].includes(parsedUrl.protocol)) {
      return false;
    }

    // Check for dangerous protocols in the URL string
    const dangerousProtocols = [
      "javascript:",
      "data:",
      "vbscript:",
      "file:",
      "ftp:",
      "chrome:",
      "about:",
    ];

    const lowerUrl = url.toLowerCase();
    for (const protocol of dangerousProtocols) {
      if (lowerUrl.includes(protocol)) {
        return false;
      }
    }

    // Basic hostname validation
    if (!parsedUrl.hostname || parsedUrl.hostname.length === 0) {
      return false;
    }

    // Check for localhost in production
    if (process.env.NODE_ENV === "production") {
      const localhostPatterns = [
        "localhost",
        "127.0.0.1",
        "0.0.0.0",
        "::1",
      ];
      
      if (localhostPatterns.some(pattern => parsedUrl.hostname.includes(pattern))) {
        return false;
      }
    }

    // Check for proper domain format
    const domainPattern = /^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*$/;
    if (!domainPattern.test(parsedUrl.hostname)) {
      return false;
    }

    return true;
  } catch {
    return false;
  }
}