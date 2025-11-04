interface SecurityValidationResult {
  allPassed: boolean;
  details: {
    csp: boolean;
    frameOptions: boolean;
    contentTypeOptions: boolean;
    referrerPolicy: boolean;
    xssProtection: boolean;
    hsts: boolean;
  };
}

export async function validateSecurityHeaders(url: string): Promise<SecurityValidationResult> {
  try {
    const response = await fetch(url, { method: "HEAD" });
    const headers = response.headers;

    const results = {
      csp: headers.get("Content-Security-Policy") !== null,
      frameOptions: headers.get("X-Frame-Options") !== null,
      contentTypeOptions: headers.get("X-Content-Type-Options") !== null,
      referrerPolicy: headers.get("Referrer-Policy") !== null,
      xssProtection: headers.get("X-XSS-Protection") !== null,
      hsts: headers.get("Strict-Transport-Security") !== null,
    };

    return {
      allPassed: Object.values(results).every(Boolean),
      details: results,
    };
  } catch (error) {
    throw new Error(
      `Security validation failed: ${error instanceof Error ? error.message : "Unknown error"}`
    );
  }
}
