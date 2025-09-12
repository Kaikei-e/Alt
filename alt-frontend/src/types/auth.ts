// Auth types for Ory Kratos integration

export interface UserPreferences {
  theme?: "light" | "dark" | "system";
  language?: string;
  notifications?: boolean;
  [key: string]: unknown;
}

export interface User {
  id: string;
  tenantId: string;
  email: string;
  name?: string;
  role: "admin" | "user" | "readonly";
  preferences?: UserPreferences;
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

export interface UINodeAttributes {
  name?: string;
  type?: string;
  value?: string;
  required?: boolean;
  disabled?: boolean;
  [key: string]: unknown;
}

export interface UINode {
  type: "input" | "img" | "a" | "script" | "text";
  group: "default" | "password" | "oidc" | "lookup_secret";
  attributes: UINodeAttributes;
  messages?: Message[];
}

export interface Message {
  id: number;
  text: string;
  type: "info" | "error" | "success";
}

export interface CSRFToken {
  token: string;
  expiresAt: string;
}
