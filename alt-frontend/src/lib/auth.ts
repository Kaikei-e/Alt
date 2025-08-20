export interface AuthValidateResponse {
  valid: boolean;
  session_id?: string;
  identity_id?: string;
}

export async function fetchAuth(): Promise<AuthValidateResponse> {
  const res = await fetch('/api/auth/validate', { 
    credentials: 'include',
    headers: {
      'Cache-Control': 'no-cache',
    }
  });
  
  if (res.status === 200) {
    const data = await res.json();
    return data;
  }
  
  if (res.status === 401) {
    return { valid: false };
  }
  
  // Other status codes are treated as service unavailable
  throw new Error(`auth validate unexpected: ${res.status}`);
}