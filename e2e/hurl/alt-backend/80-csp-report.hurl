# CSP violation endpoint — public, DOS-whitelisted, CSRF-bypassed. The
# handler accepts the browser-native CSP report shape and logs it.

POST {{base_url}}/security/csp-report
Content-Type: application/csp-report
{
  "csp-report": {
    "document-uri": "http://localhost/",
    "referrer": "",
    "violated-directive": "default-src 'none'",
    "effective-directive": "default-src",
    "original-policy": "default-src 'none'; report-uri /security/csp-report",
    "blocked-uri": "inline"
  }
}
HTTP *
[Asserts]
status >= 200
status < 500
