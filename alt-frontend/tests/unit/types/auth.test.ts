import { describe, it, expect } from "vitest";
import {
  User,
  AuthState,
  LoginFlow,
  RegistrationFlow,
  UINode,
  Message,
  CSRFToken,
} from "../../../src/types/auth";

describe("Auth Types", () => {
  describe("User interface", () => {
    it("should define required user properties", () => {
      const user: User = {
        id: "user-123",
        tenantId: "tenant-456",
        email: "test@example.com",
        role: "user",
        createdAt: "2025-01-15T10:00:00Z",
      };

      expect(user.id).toBe("user-123");
      expect(user.tenantId).toBe("tenant-456");
      expect(user.email).toBe("test@example.com");
      expect(user.role).toBe("user");
      expect(user.createdAt).toBe("2025-01-15T10:00:00Z");
    });

    it("should allow optional properties", () => {
      const userWithOptionals: User = {
        id: "user-123",
        tenantId: "tenant-456",
        email: "test@example.com",
        role: "admin",
        createdAt: "2025-01-15T10:00:00Z",
        name: "Test User",
        preferences: { theme: "dark" },
        lastLoginAt: "2025-01-15T09:00:00Z",
      };

      expect(userWithOptionals.name).toBe("Test User");
      expect(userWithOptionals.preferences).toEqual({ theme: "dark" });
      expect(userWithOptionals.lastLoginAt).toBe("2025-01-15T09:00:00Z");
    });

    it("should enforce role type safety", () => {
      // These should be valid roles
      const adminUser: User = {
        id: "1",
        tenantId: "1",
        email: "admin@example.com",
        role: "admin",
        createdAt: "2025-01-15T10:00:00Z",
      };

      const regularUser: User = {
        id: "2",
        tenantId: "1",
        email: "user@example.com",
        role: "user",
        createdAt: "2025-01-15T10:00:00Z",
      };

      const readonlyUser: User = {
        id: "3",
        tenantId: "1",
        email: "readonly@example.com",
        role: "readonly",
        createdAt: "2025-01-15T10:00:00Z",
      };

      expect(adminUser.role).toBe("admin");
      expect(regularUser.role).toBe("user");
      expect(readonlyUser.role).toBe("readonly");
    });
  });

  describe("AuthState interface", () => {
    it("should define authentication state structure", () => {
      const authState: AuthState = {
        user: null,
        isAuthenticated: false,
        isLoading: true,
        error: null,
      };

      expect(authState.user).toBeNull();
      expect(authState.isAuthenticated).toBe(false);
      expect(authState.isLoading).toBe(true);
      expect(authState.error).toBeNull();
    });

    it("should handle authenticated state", () => {
      const user: User = {
        id: "user-123",
        tenantId: "tenant-456",
        email: "test@example.com",
        role: "user",
        createdAt: "2025-01-15T10:00:00Z",
      };

      const authenticatedState: AuthState = {
        user: user,
        isAuthenticated: true,
        isLoading: false,
        error: null,
      };

      expect(authenticatedState.user).toEqual(user);
      expect(authenticatedState.isAuthenticated).toBe(true);
      expect(authenticatedState.isLoading).toBe(false);
    });

    it("should handle error state", () => {
      const errorState: AuthState = {
        user: null,
        isAuthenticated: false,
        isLoading: false,
        error: "Authentication failed",
      };

      expect(errorState.error).toBe("Authentication failed");
      expect(errorState.user).toBeNull();
      expect(errorState.isAuthenticated).toBe(false);
    });
  });

  describe("LoginFlow interface", () => {
    it("should define login flow structure", () => {
      const loginFlow: LoginFlow = {
        id: "flow-123",
        ui: {
          action: "/login",
          method: "POST",
          nodes: [],
        },
        expiresAt: "2025-01-15T11:00:00Z",
      };

      expect(loginFlow.id).toBe("flow-123");
      expect(loginFlow.ui.action).toBe("/login");
      expect(loginFlow.ui.method).toBe("POST");
      expect(loginFlow.expiresAt).toBe("2025-01-15T11:00:00Z");
    });
  });

  describe("RegistrationFlow interface", () => {
    it("should define registration flow structure", () => {
      const registrationFlow: RegistrationFlow = {
        id: "flow-456",
        ui: {
          action: "/register",
          method: "POST",
          nodes: [],
        },
        expiresAt: "2025-01-15T11:00:00Z",
      };

      expect(registrationFlow.id).toBe("flow-456");
      expect(registrationFlow.ui.action).toBe("/register");
      expect(registrationFlow.ui.method).toBe("POST");
    });
  });

  describe("UINode interface", () => {
    it("should define UI node structure with all types", () => {
      const inputNode: UINode = {
        type: "input",
        group: "default",
        attributes: {
          name: "email",
          type: "email",
          required: true,
        },
      };

      const imgNode: UINode = {
        type: "img",
        group: "oidc",
        attributes: {
          src: "/oauth/google.png",
          alt: "Google",
        },
      };

      expect(inputNode.type).toBe("input");
      expect(inputNode.group).toBe("default");
      expect(inputNode.attributes.name).toBe("email");

      expect(imgNode.type).toBe("img");
      expect(imgNode.group).toBe("oidc");
      expect(imgNode.attributes.src).toBe("/oauth/google.png");
    });

    it("should support all node types and groups", () => {
      const nodeTypes: Array<UINode["type"]> = [
        "input",
        "img",
        "a",
        "script",
        "text",
      ];
      const nodeGroups: Array<UINode["group"]> = [
        "default",
        "password",
        "oidc",
        "lookup_secret",
      ];

      // Type checking ensures these are valid
      expect(nodeTypes).toContain("input");
      expect(nodeGroups).toContain("default");
    });
  });

  describe("Message interface", () => {
    it("should define message structure", () => {
      const errorMessage: Message = {
        id: 4000001,
        text: "The provided credentials are invalid.",
        type: "error",
      };

      const successMessage: Message = {
        id: 1000001,
        text: "Registration successful.",
        type: "success",
      };

      const infoMessage: Message = {
        id: 1000002,
        text: "Please check your email.",
        type: "info",
      };

      expect(errorMessage.type).toBe("error");
      expect(successMessage.type).toBe("success");
      expect(infoMessage.type).toBe("info");
    });
  });

  describe("CSRFToken interface", () => {
    it("should define CSRF token structure", () => {
      const csrfToken: CSRFToken = {
        token: "csrf-token-123",
        expiresAt: "2025-01-15T11:00:00Z",
      };

      expect(csrfToken.token).toBe("csrf-token-123");
      expect(csrfToken.expiresAt).toBe("2025-01-15T11:00:00Z");
    });
  });
});
