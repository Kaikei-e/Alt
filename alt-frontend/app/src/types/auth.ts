// Auth types for Ory Kratos integration

export interface User {
  id: string;
  tenantId: string;
  email: string;
  name?: string;
  role: 'admin' | 'user' | 'readonly';
  preferences?: Record<string, any>;
  createdAt: string;
  lastLoginAt?: string;
}

export interface AuthState {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  error: string | null;
}

export interface LoginFlow {
  id: string;
  ui: UIContainer;
  expiresAt: string;
}

export interface RegistrationFlow {
  id: string;
  ui: UIContainer;
  expiresAt: string;
}

export interface UIContainer {
  action: string;
  method: string;
  nodes: UINode[];
  messages?: Message[];
}

export interface UINode {
  type: 'input' | 'img' | 'a' | 'script' | 'text';
  group: 'default' | 'password' | 'oidc' | 'lookup_secret';
  attributes: Record<string, any>;
  messages?: Message[];
}

export interface Message {
  id: number;
  text: string;
  type: 'info' | 'error' | 'success';
}

export interface CSRFToken {
  token: string;
  expiresAt: string;
}