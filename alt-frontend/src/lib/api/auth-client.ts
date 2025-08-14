import type { User, LoginFlow, RegistrationFlow, UserPreferences } from '@/types/auth';

export class AuthAPIClient {
  private baseURL: string;

  constructor() {
    // Use relative API proxy endpoints for secure HTTPS communication
    // This avoids mixed content issues and keeps internal URLs secure
    this.baseURL = '/api/auth';
  }

  // 接続テスト機能追加 (X1.md 1.3.2 実装)
  async testConnection(): Promise<boolean> {
    try {
      const response = await fetch(`${this.baseURL}`, {
        method: 'GET',
        signal: AbortSignal.timeout(5000)
      });
      return response.ok;
    } catch {
      return false;
    }
  }

  async initiateLogin(): Promise<LoginFlow> {
    const response = await this.makeRequest('POST', '/login');
    
    // 防御的プログラミング: レスポンスデータの検証
    if (!response || !response.data || typeof response.data !== 'object') {
      throw new Error('Invalid login flow response format');
    }
    
    const loginFlow = response.data as LoginFlow;
    if (!loginFlow.id) {
      throw new Error('Login flow response missing required ID');
    }
    
    return loginFlow;
  }

  async completeLogin(flowId: string, email: string, password: string): Promise<User> {
    // Kratos login形式に変換
    const payload = {
      identifier: email,
      password: password,
      method: 'password'
    };

    const response = await this.makeRequest('POST', `/login/${flowId}`, payload);
    return response.data as User;
  }

  async initiateRegistration(): Promise<RegistrationFlow> {
    const response = await this.makeRequest('POST', '/register');
    
    // 防御的プログラミング: レスポンスデータの検証
    if (!response || !response.data || typeof response.data !== 'object') {
      throw new Error('Invalid registration flow response format');
    }
    
    const registrationFlow = response.data as RegistrationFlow;
    if (!registrationFlow.id) {
      throw new Error('Registration flow response missing required ID');
    }
    
    return registrationFlow;
  }

  async completeRegistration(flowId: string, email: string, password: string, name?: string): Promise<User> {
    // Kratos traits形式に変換
    const payload = {
      traits: {
        email: email,
        name: name ? {
          first: name.split(' ')[0] || '',
          last: name.split(' ').slice(1).join(' ') || ''
        } : undefined
      },
      password: password,
      method: 'password'
    };

    // undefinedフィールドを除去
    if (!payload.traits.name) {
      delete payload.traits.name;
    }

    const response = await this.makeRequest('POST', `/register/${flowId}`, payload);
    return response.data as User;
  }

  async logout(): Promise<void> {
    await this.makeRequest('POST', '/logout');
  }

  async getCurrentUser(): Promise<User | null> {
    try {
      const url = `${this.baseURL}/validate`;
      const response = await fetch(url, {
        method: 'GET',
        credentials: 'include',
      });

      if (response.status === 401) {
        return null; // Not authenticated
      }

      if (!response.ok) {
        throw new Error(this.getMethodDescription('GET', '/validate'));
      }

      const data = await response.json();
      return data.data as User;
    } catch (error: unknown) {
      if (error instanceof Error && error.message && (error.message.includes('401') || error.message.includes('Unauthorized'))) {
        return null; // Not authenticated
      }
      throw error;
    }
  }

  async getCSRFToken(): Promise<string | null> {
    try {
      const response = await this.makeRequest('POST', '/csrf');
      
      // 防御的プログラミング: CSRF レスポンスの検証強化
      if (!response || !response.data || typeof response.data !== 'object') {
        console.warn('CSRF response invalid format:', response);
        return null;
      }
      
      const csrfData = response.data as { csrf_token?: string };
      if (!csrfData.csrf_token || typeof csrfData.csrf_token !== 'string') {
        console.warn('CSRF response missing token:', csrfData);
        return null;
      }
      
      return csrfData.csrf_token;
    } catch (error: unknown) {
      console.warn('Failed to get CSRF token:', error);
      return null;
    }
  }

  async updateProfile(profile: Partial<User>): Promise<User> {
    const response = await this.makeRequest('PUT', '/profile', profile);
    return response.data as User;
  }

  async getUserSettings(): Promise<UserPreferences> {
    const response = await this.makeRequest('GET', '/settings');
    return response.data as UserPreferences;
  }

  async updateUserSettings(settings: UserPreferences): Promise<void> {
    await this.makeRequest('PUT', '/settings', settings);
  }

  private async makeRequest(method: string, endpoint: string, body?: unknown): Promise<{ data: unknown }> {
    const url = `${this.baseURL}${endpoint}`;
    const isUnsafeMethod = ['POST', 'PUT', 'PATCH', 'DELETE'].includes(method.toUpperCase());
    const isCsrfEndpoint = endpoint.includes('/csrf');

    const headers: Record<string, string> = {};

    // Add CSRF token for unsafe methods (but not for CSRF endpoint itself to avoid circular dependency)
    if (isUnsafeMethod && !isCsrfEndpoint) {
      const csrfToken = await this.getCSRFTokenInternal();
      if (csrfToken) {
        headers['X-CSRF-Token'] = csrfToken;
      }
    }

    // Add content type for requests with body
    if (body) {
      headers['Content-Type'] = 'application/json';
    }

    const config: RequestInit = {
      method,
      credentials: 'include', // Include cookies
      headers,
    };

    if (body) {
      config.body = JSON.stringify(body);
    }

    try {
      const response = await fetch(url, config);

      if (!response.ok) {
        const errorContext = this.getMethodDescription(method, endpoint);
        const error = new Error(`HTTP ${response.status}: ${errorContext}`);
        throw this.handleError(error, errorContext);
      }

      return await response.json();
    } catch (error) {
      const errorContext = this.getMethodDescription(method, endpoint);
      throw this.handleError(error, errorContext);
    }
  }

  private async getCSRFTokenInternal(): Promise<string | null> {
    try {
      const url = `${this.baseURL}/csrf`;
      const response = await fetch(url, {
        method: 'POST',
        credentials: 'include',
      });

      if (!response.ok) {
        return null;
      }

      const data = await response.json();
      return data.data?.csrf_token || null;
    } catch {
      return null;
    }
  }

  // 詳細エラーログ追加 (X1.md 1.3.2 実装)
  private handleError(error: unknown, context: string): Error {
    console.error(`Auth API Error [${context}]:`, error);
    
    if (error instanceof Error) {
      return new Error(`${context}: ${error.message}`);
    }
    
    return new Error(`${context}: Unknown error occurred`);
  }

  private getMethodDescription(method: string, endpoint: string): string {
    if (endpoint.includes('/login') && method === 'POST' && !endpoint.includes('/login/')) {
      return 'Failed to initiate login';
    }
    if (endpoint.includes('/login/') && method === 'POST') {
      return 'Failed to complete login';
    }
    if (endpoint.includes('/register') && method === 'POST' && !endpoint.includes('/register/')) {
      return 'Failed to initiate registration';
    }
    if (endpoint.includes('/register/') && method === 'POST') {
      return 'Failed to complete registration';
    }
    if (endpoint.includes('/logout')) {
      return 'Failed to logout';
    }
    if (endpoint.includes('/validate')) {
      return 'Failed to get current user';
    }
    if (endpoint.includes('/csrf')) {
      return 'Failed to get CSRF token';
    }
    if (endpoint.includes('/profile')) {
      return 'Failed to update profile';
    }
    if (endpoint.includes('/settings')) {
      return method === 'GET' ? 'Failed to get user settings' : 'Failed to update user settings';
    }
    return `Request failed`;
  }
}

// Export singleton instance
export const authAPI = new AuthAPIClient();