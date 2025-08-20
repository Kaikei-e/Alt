// Build-time required public envs
const required = [
  'NEXT_PUBLIC_IDP_ORIGIN',
  'NEXT_PUBLIC_KRATOS_PUBLIC_URL',
];

for (const k of required) {
  const v = process.env[k];
  if (!v) {
    throw new Error(`[ENV] ${k} is required at build time`);
  }
  try {
    const origin = new URL(v).origin;
    if (!origin.startsWith('https://')) {
      throw new Error(`[ENV] ${k} must be HTTPS origin (got: ${origin})`);
    }
    const pattern = new RegExp('\\.' + 'cluster' + '\\.' + 'local' + '(\\b|:|\/)', 'i');
    if (pattern.test(origin)) {
      throw new Error(`[ENV] ${k} must be PUBLIC FQDN (got: ${origin})`);
    }
  } catch {
    throw new Error(`[ENV] ${k} must be a valid URL (got: ${v})`);
  }
}

console.log('[ENV] Public envs validated:', required.join(', '));

