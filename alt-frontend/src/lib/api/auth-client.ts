import type { User, LoginFlow, RegistrationFlow, UserPreferences } from '@/types/auth';

export class AuthAPIClient {
  private baseURL: string;
  private debugMode: boolean;
  private requestId: number;

  constructor() {
    // Use relative API proxy endpoints for secure HTTPS communication
    // This avoids mixed content issues and keeps internal URLs secure
    this.baseURL = '/api/auth';
    this.debugMode = process.env.NODE_ENV === 'development';
    this.requestId = 0;
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
    return this.loginWithBrowserFlow(flowId, email, password);
  }

  // 🚨 LEGACY: X22 Auto-retry login (replaced by X27 Browser Flow)
  // Kept for backward compatibility - will be removed in future versions
  private async loginWithRetry(flowId: string, email: string, password: string, maxRetries: number = 2): Promise<User> {
    console.warn('⚠️ DEPRECATED: loginWithRetry is deprecated, using Browser Flow instead');
    return this.loginWithBrowserFlow(flowId, email, password);
  }

  // 🚨 CRITICAL: X22 Phase 1 - CSRF error detection
  private isCSRFError(error: unknown): boolean {
    if (!(error instanceof Error)) return false;
    
    const message = error.message.toLowerCase();
    return message.includes('csrf') || 
           message.includes('token') ||
           message.includes('400') ||
           message.includes('500') ||
           message.includes('forbidden');
  }

  // 🚀 X27 Browser Flow Methods - Ory Kratos Compliance

  // Browser Flow compliant login method
  private async loginWithBrowserFlow(flowId: string, email: string, password: string): Promise<User> {
    console.log('🚀 Starting Browser Flow login...', { flowId });

    try {
      // Get login flow to extract ui.action
      const loginFlow = await this.getLoginFlow(flowId);
      
      if (!loginFlow.ui.action) {
        throw new Error('Login flow missing ui.action URL');
      }

      // Create form data for Browser Flow
      const formData = this.createLoginFormData(email, password, loginFlow);

      console.log('[AUTH-CLIENT] Sending Browser Flow login:', {
        flowId: flowId,
        actionUrl: loginFlow.ui.action,
        method: loginFlow.ui.method,
        formFields: Array.from(formData.keys())
      });

      // Submit to ui.action URL using form data
      const response = await this.submitBrowserFlowForm(loginFlow.ui.action, formData);
      
      console.log('✅ [AUTH-CLIENT] Browser Flow Login SUCCESS');
      
      return response.data as User;
      
    } catch (error) {
      console.error('❌ [AUTH-CLIENT] Browser Flow Login FAILED:', error);
      throw error;
    }
  }

  // Get login flow by ID to extract ui.action
  private async getLoginFlow(flowId: string): Promise<LoginFlow> {
    const response = await this.makeRequest('GET', `/login/${flowId}`);
    
    if (!response || !response.data || typeof response.data !== 'object') {
      throw new Error('Invalid login flow response format');
    }

    const loginFlow = response.data as LoginFlow;
    if (!loginFlow.ui || !loginFlow.ui.action) {
      throw new Error('Login flow missing ui.action field');
    }

    return loginFlow;
  }

  // Create form data for login (Browser Flow)
  private createLoginFormData(email: string, password: string, flow: LoginFlow): FormData {
    const formData = new FormData();
    
    // Required fields for Kratos login
    formData.append('method', 'password');
    formData.append('identifier', email.trim().toLowerCase());
    formData.append('password', password.trim());

    // Extract and add CSRF token from flow UI nodes
    const csrfNode = flow.ui.nodes.find(node => 
      node.attributes?.name === 'csrf_token' && node.attributes?.value
    );
    
    if (csrfNode?.attributes?.value) {
      formData.append('csrf_token', csrfNode.attributes.value);
    }

    return formData;
  }

  // Get registration flow by ID to extract ui.action
  private async getRegistrationFlow(flowId: string): Promise<RegistrationFlow> {
    const response = await this.makeRequest('GET', `/register/${flowId}`);
    
    if (!response || !response.data || typeof response.data !== 'object') {
      throw new Error('Invalid registration flow response format');
    }

    const registrationFlow = response.data as RegistrationFlow;
    if (!registrationFlow.ui || !registrationFlow.ui.action) {
      throw new Error('Registration flow missing ui.action field');
    }

    return registrationFlow;
  }

  // Create form data for Browser Flow (not JSON)
  private createBrowserFlowFormData(email: string, password: string, name: string | undefined, flow: RegistrationFlow): FormData {
    const formData = new FormData();
    
    // Required fields for Kratos registration
    formData.append('method', 'password');
    formData.append('password', password.trim());
    formData.append('traits.email', email.trim().toLowerCase());
    
    // Add name if provided
    if (name && name.trim()) {
      const nameParts = name.trim().split(/\s+/);
      formData.append('traits.name.first', nameParts[0] || '');
      if (nameParts.length > 1) {
        formData.append('traits.name.last', nameParts.slice(1).join(' '));
      }
    }

    // Extract and add CSRF token from flow UI nodes
    const csrfNode = flow.ui.nodes.find(node => 
      node.attributes?.name === 'csrf_token' && node.attributes?.value
    );
    
    if (csrfNode?.attributes?.value) {
      formData.append('csrf_token', csrfNode.attributes.value);
    }

    return formData;
  }

  // Submit form data to ui.action URL (Browser Flow compliance)
  private async submitBrowserFlowForm(actionUrl: string, formData: FormData): Promise<{ data: unknown }> {
    const response = await fetch(actionUrl, {
      method: 'POST',
      body: formData,
      credentials: 'include', // Critical for cookie-based sessions
      headers: {
        // Do NOT set Content-Type - let browser set it for FormData
        'Accept': 'application/json',
        'X-Requested-With': 'XMLHttpRequest',
      }
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Browser Flow submission failed: ${response.status} ${response.statusText} - ${errorText}`);
    }

    const data = await response.json();
    return { data };
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
    console.log('🚀 Starting Browser Flow registration completion...', { flowId });

    // Basic validation
    if (!flowId) {
      throw new Error('Flow ID is required');
    }
    if (!email || !email.includes('@')) {
      throw new Error('Valid email address is required');
    }
    if (!password || password.length < 8) {
      throw new Error('Password must be at least 8 characters');
    }

    try {
      // 🚀 CRITICAL: X27 Browser Flow Compliance - Get registration flow to extract ui.action
      const registrationFlow = await this.getRegistrationFlow(flowId);
      
      if (!registrationFlow.ui.action) {
        throw new Error('Registration flow missing ui.action URL');
      }

      // 🎯 Browser Flow Compliance: Create form data payload instead of JSON
      const formData = this.createBrowserFlowFormData(email, password, name, registrationFlow);

      console.log('[AUTH-CLIENT] Sending Browser Flow registration:', {
        flowId: flowId,
        actionUrl: registrationFlow.ui.action,
        method: registrationFlow.ui.method,
        formFields: Array.from(formData.keys())
      });

      // 🚀 CRITICAL: Submit to ui.action URL using form data (not JSON)
      const response = await this.submitBrowserFlowForm(registrationFlow.ui.action, formData);
      
      console.log('✅ [AUTH-CLIENT] Browser Flow Registration SUCCESS');
      
      return response.data as User;
      
    } catch (error) {
      console.error('❌ [AUTH-CLIENT] Browser Flow Registration FAILED:', error);
      throw error;
    }
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

  // 🚨 CRITICAL: X22 Phase 1 - CSRF token extraction from login flow
  private async extractCSRFTokenFromFlow(flowId: string): Promise<string | null> {
    try {
      // Get current flow to extract CSRF token
      const flowResponse = await fetch(`${this.baseURL}/login/${flowId}`, {
        method: 'GET',
        credentials: 'include', // 🔑 Essential for cookie transmission
      });

      if (!flowResponse.ok) {
        console.error('🚨 Failed to fetch login flow for CSRF extraction:', {
          status: flowResponse.status,
          statusText: flowResponse.statusText,
          flowId
        });
        return null;
      }

      const flow = await flowResponse.json();
      
      // Extract CSRF token from UI nodes
      const csrfToken = this.extractCSRFTokenFromUINodes(flow);
      
      if (!csrfToken) {
        console.error('🚨 CSRF token not found in login flow', {
          flowId,
          available_nodes: flow.ui?.nodes?.map((n: any) => n.attributes?.name) || [],
          flow_preview: {
            id: flow.id,
            type: flow.type,
            state: flow.state,
            nodes_count: flow.ui?.nodes?.length || 0
          }
        });
        return null;
      }

      console.log('✅ CSRF token extracted successfully from flow', {
        flowId,
        token_length: csrfToken.length,
        token_preview: `${csrfToken.substring(0, 8)}...${csrfToken.substring(csrfToken.length - 8)}`
      });

      return csrfToken;
    } catch (error) {
      console.error('🚨 Error extracting CSRF token from flow:', {
        flowId,
        error: error instanceof Error ? error.message : String(error)
      });
      return null;
    }
  }

  // 🚨 CRITICAL: X22 Phase 1 - CSRF token extraction from UI nodes
  private extractCSRFTokenFromUINodes(flow: any): string | null {
    if (!flow?.ui?.nodes || !Array.isArray(flow.ui.nodes)) {
      console.warn('Invalid flow structure - missing ui.nodes');
      return null;
    }

    // Find CSRF token node
    const csrfNode = flow.ui.nodes.find((node: any) => 
      node?.attributes?.name === 'csrf_token' && 
      node?.attributes?.type === 'hidden'
    );

    if (!csrfNode?.attributes?.value) {
      console.warn('CSRF token node not found or missing value', {
        available_nodes: flow.ui.nodes.map((n: any) => ({
          name: n?.attributes?.name,
          type: n?.attributes?.type,
          group: n?.group
        }))
      });
      return null;
    }

    return csrfNode.attributes.value;
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




  private createKratosCompliantRegistrationPayload(email: string, password: string, name?: string): any {
    const payload: any = {
      method: "password",
      password: password.trim(),
      traits: {
        email: email.trim().toLowerCase()
      }
    };

    // Add name if provided
    if (name && name.trim()) {
      const nameParts = name.trim().split(/\s+/);
      payload.traits.name = {
        first: nameParts[0] || "",
        last: nameParts.slice(1).join(" ") || ""
      };
      
      // Remove empty last name
      if (!payload.traits.name.last) {
        delete payload.traits.name.last;
      }
    }

    return payload;
  }

  private createKratosCompliantLoginPayload(email: string, password: string, csrfToken: string): any {
    return {
      method: "password",
      identifier: email.trim().toLowerCase(),
      password: password.trim(),
      csrf_token: csrfToken  // 🔑 CRITICAL: CSRF token inclusion
    };
  }


  private async makeRequest(method: string, endpoint: string, body?: unknown): Promise<{ data: unknown }> {
    const url = `${this.baseURL}${endpoint}`;
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };

    // 🚀 X26 Phase 2: Enhanced S2S authentication headers for auth-service compatibility
    // Following Ory Kratos official recommendations for service-to-service communication

    // Add CSRF token for unsafe methods (except CSRF endpoint to avoid circular dependency)
    const isUnsafeMethod = ['POST', 'PUT', 'PATCH', 'DELETE'].includes(method.toUpperCase());
    const isCsrfEndpoint = endpoint.includes('/csrf');
    
    if (isUnsafeMethod && !isCsrfEndpoint) {
      const csrfToken = await this.getCSRFTokenInternal();
      if (csrfToken) {
        headers['X-CSRF-Token'] = csrfToken;
        // 🔑 Ory Kratos recommended: X-Session-Token for S2S auth reliability
        headers['X-Session-Token'] = csrfToken;
      }
    }

    // 🚀 X26 Phase 2: Additional headers for enhanced auth-service compatibility
    headers['X-Requested-With'] = 'XMLHttpRequest';
    headers['X-Client-Type'] = 'frontend-spa';
    
    // 🔑 Essential for CSRF endpoint direct routing
    if (isCsrfEndpoint) {
      headers['X-Auth-Flow'] = 'csrf-request';
      headers['X-Internal-Request'] = 'true';
    }

    const config: RequestInit = {
      method,
      credentials: 'include', // 🔑 CRITICAL: Always include credentials for Kratos session cookies
      headers,
    };

    if (body) {
      config.body = JSON.stringify(body);
    }

    try {
      const response = await fetch(url, config);

      if (!response.ok) {
        const error = new Error(`HTTP ${response.status}: ${method} ${endpoint}`);
        throw this.handleError(error, `${method} ${endpoint}`);
      }

      return await response.json();
    } catch (error) {
      throw this.handleError(error, `${method} ${endpoint}`);
    }
  }

  private async getCSRFTokenInternal(): Promise<string | null> {
    try {
      const url = `${this.baseURL}/csrf`;
      
      // 🚀 X26 Phase 2: Enhanced CSRF request with proper headers for direct auth-service routing
      const response = await fetch(url, {
        method: 'POST',
        credentials: 'include', // 🔑 Essential for session cookie transmission
        headers: {
          'Content-Type': 'application/json',
          'X-Auth-Flow': 'csrf-request',
          'X-Internal-Request': 'true',
          'X-Requested-With': 'XMLHttpRequest',
          'X-Client-Type': 'frontend-spa',
        },
      });

      if (!response.ok) {
        console.error('🚨 CSRF token request failed:', {
          status: response.status,
          statusText: response.statusText,
          url,
          headers: Object.fromEntries(response.headers.entries())
        });
        return null;
      }

      const data = await response.json();
      const token = data.data?.csrf_token || data.csrf_token || null;
      
      if (token) {
        console.log('✅ CSRF token retrieved successfully via direct auth-service route');
      } else {
        console.warn('⚠️ CSRF response received but no token found:', data);
      }
      
      return token;
    } catch (error) {
      console.error('🚨 CSRF token request error:', error);
      return null;
    }
  }

  // 詳細エラー分析とKratos固有エラー変換
  private handleError(error: unknown, context: string): Error {
    // 詳細診断ログ
    console.error(`Auth API Error [${context}]:`, error);
    console.error(`Auth API Error [${context}] - Type:`, typeof error);

    if (error instanceof Error) {
      console.error(`Auth API Error [${context}] - Message:`, error.message);
      console.error(`Auth API Error [${context}] - Name:`, error.name);

      // Kratos固有エラーの判定と適切な変換
      if (error.message.includes('Property email is missing')) {
        return new Error(`VALIDATION_FAILED: Property email is missing - ${context}`);
      }

      if (error.message.includes('already registered') || error.message.includes('User already exists')) {
        return new Error(`USER_ALREADY_EXISTS: User already exists - ${context}`);
      }

      if (error.message.includes('flow expired') || error.message.includes('410')) {
        return new Error(`FLOW_EXPIRED: Registration flow expired - ${context}`);
      }

      if (error.message.includes('502') || error.message.includes('503')) {
        return new Error(`KRATOS_SERVICE_ERROR: Authentication service unavailable - ${context}`);
      }

      // HTTPステータスコード別の処理
      if (error.message.includes('HTTP 400')) {
        return new Error(`VALIDATION_FAILED: Bad request - ${context}: ${error.message}`);
      }

      // 🚨 FIX: HTTP 401 専用ハンドリング追加
      if (error.message.includes('HTTP 401')) {
        return new Error(`SESSION_NOT_FOUND: Authentication required - ${context}: ${error.message}`);
      }

      // 🚨 FIX: HTTP 404 専用ハンドリング追加
      if (error.message.includes('HTTP 404')) {
        return new Error(`KRATOS_SERVICE_ERROR: Authentication endpoint not found - ${context}: ${error.message}`);
      }

      if (error.message.includes('HTTP 409')) {
        return new Error(`USER_ALREADY_EXISTS: Conflict - ${context}: ${error.message}`);
      }

      if (error.message.includes('HTTP 410')) {
        return new Error(`FLOW_EXPIRED: Gone - ${context}: ${error.message}`);
      }

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