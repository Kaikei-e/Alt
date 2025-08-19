// instrumentation.ts - Next.js App Router instrumentation hook
// TODO.md: 最低限のサーバログ捕捉でDigestの裏のスタックを可視化

export async function register() {
  // Unhandled Promise rejections
  process.on('unhandledRejection', (reason, promise) => {
    console.error('[unhandledRejection]', {
      reason: reason instanceof Error ? reason.message : reason,
      stack: reason instanceof Error ? reason.stack : undefined,
      promise
    })
  })

  // Uncaught exceptions
  process.on('uncaughtException', (error) => {
    console.error('[uncaughtException]', {
      message: error.message,
      stack: error.stack,
      name: error.name
    })
  })

  // Warning events
  process.on('warning', (warning) => {
    console.warn('[warning]', {
      name: warning.name,
      message: warning.message,
      stack: warning.stack
    })
  })

  // Increase stack trace limit for better debugging
  Error.stackTraceLimit = 50

  console.log('[instrumentation] Error handlers registered for Next.js App Router')
}