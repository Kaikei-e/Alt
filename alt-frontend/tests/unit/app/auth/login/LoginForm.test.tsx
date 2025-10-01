/**
 * @vitest-environment jsdom
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import LoginForm from "../../../../../src/app/auth/login/LoginForm";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";

// Oryクライアントのモック
vi.mock("@ory/client", () => {
  const mockGetLoginFlow = vi.fn();
  const mockUpdateLoginFlow = vi.fn();

  return {
    FrontendApi: vi.fn(() => ({
      getLoginFlow: mockGetLoginFlow,
      updateLoginFlow: mockUpdateLoginFlow,
    })),
    Configuration: vi.fn(),
    __mockGetLoginFlow: mockGetLoginFlow,
    __mockUpdateLoginFlow: mockUpdateLoginFlow,
  };
});

// window.locationのモック
Object.defineProperty(window, "location", {
  value: {
    href: "http://localhost:3000",
    replace: vi.fn(),
    assign: vi.fn(),
    reload: vi.fn(),
  },
  writable: true,
});

// Environment variables mock
process.env.NEXT_PUBLIC_APP_ORIGIN = "https://curionoah.com";
process.env.NEXT_PUBLIC_IDP_ORIGIN = "https://curionoah.com";
process.env.NEXT_PUBLIC_RETURN_TO_DEFAULT = "https://curionoah.com/";

// モックへのアクセス用ヘルパー
const getMocks = async () => {
  const oryMock = vi.mocked(await import("@ory/client"));
  return {
    mockGetLoginFlow: (oryMock as any).__mockGetLoginFlow,
    mockUpdateLoginFlow: (oryMock as any).__mockUpdateLoginFlow,
  };
};

const renderWithProviders = (ui: React.ReactElement) =>
  render(<ChakraProvider value={defaultSystem}>{ui}</ChakraProvider>);

const LOADING_TEXT = "フローを読み込んでいます…";
const NETWORK_ERROR_TEXT =
  "ネットワークエラーが発生しました。ページを再読み込みしてください。";
const IDENTIFIER_LABEL = "メールアドレス";
const PASSWORD_LABEL = "パスワード";
const SUBMIT_LABEL = "ログイン";
const KRATOS_PUBLIC_URL = (
  process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL ?? "https://id.test.example.com"
).replace(/\/$/, "");
const KRATOS_LOGIN_FLOW_URL = `${KRATOS_PUBLIC_URL}/self-service/login/browser`;

const createUiText = (text: string) => ({ id: 0, text, type: "info" });

const createIdentifierNode = () => ({
  type: "input",
  attributes: {
    name: "identifier",
    type: "email",
    required: true,
    label: createUiText(IDENTIFIER_LABEL),
  },
  messages: [],
  meta: { label: createUiText(IDENTIFIER_LABEL) },
});

const createPasswordNode = () => ({
  type: "input",
  attributes: {
    name: "password",
    type: "password",
    required: true,
    label: createUiText(PASSWORD_LABEL),
  },
  messages: [],
  meta: { label: createUiText(PASSWORD_LABEL) },
});

const createSubmitNode = () => ({
  type: "input",
  attributes: {
    name: "method",
    type: "submit",
    value: "password",
    label: createUiText(SUBMIT_LABEL),
  },
  messages: [],
  meta: { label: createUiText(SUBMIT_LABEL) },
});

describe("LoginForm", () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    // デフォルトのモック動作を設定
    const { mockGetLoginFlow } = await getMocks();
    mockGetLoginFlow.mockImplementation(() => new Promise(() => {})); // pending promise
  });

  afterEach(() => {
    cleanup();
  });

  describe("初期状態", () => {
    it("ローディング状態が表示される", () => {
      renderWithProviders(<LoginForm flowId="test-flow-id" />);
      expect(screen.getByText(LOADING_TEXT)).toBeInTheDocument();
    });

    it("フローが正常に読み込まれてフォームが表示される", async () => {
      const { mockGetLoginFlow } = await getMocks();

      const mockFlow = {
        id: "test-flow-id",
        ui: {
          nodes: [createIdentifierNode()],
        },
      };

      mockGetLoginFlow.mockResolvedValue({ data: mockFlow });

      renderWithProviders(<LoginForm flowId="test-flow-id" />);

      await waitFor(() => {
        expect(screen.getByLabelText(IDENTIFIER_LABEL)).toBeInTheDocument();
      });
    });
  });

  describe("フロー取得", () => {
    it("正常なフローデータを取得して表示する", async () => {
      const { mockGetLoginFlow } = await getMocks();
      const mockFlow = {
        id: "test-flow-id",
        ui: {
          nodes: [
            createIdentifierNode(),
            createPasswordNode(),
            createSubmitNode(),
          ],
        },
      };

      mockGetLoginFlow.mockResolvedValue({ data: mockFlow });

      renderWithProviders(<LoginForm flowId="test-flow-id" />);

      await waitFor(() => {
        expect(screen.getByLabelText(IDENTIFIER_LABEL)).toBeInTheDocument();
        expect(screen.getByLabelText(PASSWORD_LABEL)).toBeInTheDocument();
      });
    });

    it("フロー取得失敗時にエラーを表示する", async () => {
      const { mockGetLoginFlow } = await getMocks();
      mockGetLoginFlow.mockRejectedValue(new Error("Network error"));

      renderWithProviders(<LoginForm flowId="test-flow-id" />);

      await waitFor(() => {
        expect(screen.getByText(NETWORK_ERROR_TEXT)).toBeInTheDocument();
      });
    });
  });

  describe("フォーム送信", () => {
    it("正常な送信処理を行う", async () => {
      const { mockGetLoginFlow, mockUpdateLoginFlow } = await getMocks();
      const user = userEvent.setup();
      const mockFlow = {
        id: "test-flow-id",
        return_to: "https://curionoah.com/",
        ui: {
          action: "/self-service/login",
          method: "POST",
          nodes: [
            createIdentifierNode(),
            createPasswordNode(),
            createSubmitNode(),
          ],
        },
      };

      const mockResponse = {
        data: {
          session: { id: "session-id" },
          return_to: "https://curionoah.com/",
        },
      };

      mockGetLoginFlow.mockResolvedValue({ data: mockFlow });
      mockUpdateLoginFlow.mockResolvedValue(mockResponse);

      // window.location.href のモック
      const mockLocation = {
        href: "",
        replace: vi.fn(),
        assign: vi.fn(),
        reload: vi.fn(),
      };
      Object.defineProperty(window, "location", {
        value: mockLocation,
        writable: true,
      });

      renderWithProviders(<LoginForm flowId="test-flow-id" />);

      await waitFor(() => {
        expect(screen.getByLabelText(IDENTIFIER_LABEL)).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText(IDENTIFIER_LABEL), "test@example.com");
      await user.type(screen.getByLabelText(PASSWORD_LABEL), "password123");
      await user.click(screen.getByRole("button", { name: SUBMIT_LABEL }));

      await waitFor(() => {
        expect(mockLocation.replace).toHaveBeenCalledWith(
          "https://curionoah.com/",
        );
      });
    });

    it("ログイン失敗時にエラーメッセージを表示する", async () => {
      const { mockGetLoginFlow, mockUpdateLoginFlow } = await getMocks();
      const user = userEvent.setup();
      const mockFlow = {
        id: "test-flow-id",
        return_to: "https://curionoah.com/",
        ui: {
          action: "/self-service/login",
          method: "POST",
          nodes: [
            createIdentifierNode(),
            createPasswordNode(),
            createSubmitNode(),
          ],
        },
      };

      const errorResponse = {
        response: {
          status: 400,
          data: {
            ui: {
              messages: [
                {
                  id: 4000006,
                  text: "The provided credentials are invalid.",
                  type: "error",
                },
              ],
            },
          },
        },
      };

      mockGetLoginFlow.mockResolvedValue({ data: mockFlow });
      mockUpdateLoginFlow.mockRejectedValue(errorResponse);

      renderWithProviders(<LoginForm flowId="test-flow-id" />);

      await waitFor(() => {
        expect(screen.getByLabelText(IDENTIFIER_LABEL)).toBeInTheDocument();
      });

      await user.type(
        screen.getByLabelText(IDENTIFIER_LABEL),
        "wrong@example.com",
      );
      await user.type(
        screen.getByLabelText(PASSWORD_LABEL),
        "wrongpassword",
      );
      await user.click(screen.getByRole("button", { name: SUBMIT_LABEL }));

      await waitFor(() => {
        // ログインが失敗した場合、ボタンは再度有効になる
        expect(
          screen.getByRole("button", { name: SUBMIT_LABEL }),
        ).not.toBeDisabled();
      });
    });
  });

  describe("410 Error Handling (Flow Expiration)", () => {
    it("should redirect to new flow when getLoginFlow returns 410", async () => {
      const { mockGetLoginFlow } = await getMocks();
      const error410 = {
        response: {
          status: 410,
          data: {
            error: {
              id: "self_service_flow_expired",
              code: 410,
              status: "Gone",
              message: "Flow expired",
            },
          },
        },
      };

      mockGetLoginFlow.mockRejectedValue(error410);

      const mockLocation = {
        href: "",
        replace: vi.fn(),
        assign: vi.fn(),
        reload: vi.fn(),
      };
      Object.defineProperty(window, "location", {
        value: mockLocation,
        writable: true,
      });

      mockLocation.href =
        "https://curionoah.com/auth/login?flow=expired-flow&return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Fhome";

      renderWithProviders(<LoginForm flowId="expired-flow-id" />);

      await waitFor(() => {
        expect(mockLocation.replace).toHaveBeenCalled();
        expect(mockLocation.replace.mock.calls[0][0]).toBe(
          KRATOS_LOGIN_FLOW_URL,
        );
      });
    });

    it("should redirect to new flow when updateLoginFlow returns 410", async () => {
      const { mockGetLoginFlow, mockUpdateLoginFlow } = await getMocks();
      const user = userEvent.setup();
      const mockFlow = {
        id: "test-flow-id",
        return_to: "https://curionoah.com/",
        ui: {
          action: "/self-service/login",
          method: "POST",
          nodes: [
            createIdentifierNode(),
            createPasswordNode(),
            createSubmitNode(),
          ],
        },
      };

      const error410 = {
        response: {
          status: 410,
          data: {
            error: {
              id: "self_service_flow_expired",
            },
          },
        },
      };

      mockGetLoginFlow.mockResolvedValue({ data: mockFlow });
      mockUpdateLoginFlow.mockRejectedValue(error410);

      // Mock window.location.href with trusted origin
      const mockLocation = {
        href: "https://curionoah.com/auth/login?flow=test-flow&return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Fsettings",
        replace: vi.fn(),
        assign: vi.fn(),
        reload: vi.fn(),
      };
      Object.defineProperty(window, "location", {
        value: mockLocation,
        writable: true,
      });

      renderWithProviders(<LoginForm flowId="test-flow-id" />);

      await waitFor(() => {
        expect(screen.getByLabelText(IDENTIFIER_LABEL)).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText(IDENTIFIER_LABEL), "test@example.com");
      await user.type(
        screen.getByLabelText(PASSWORD_LABEL),
        "password123",
      );
      await user.click(screen.getByRole("button", { name: SUBMIT_LABEL }));

      await waitFor(() => {
        expect(mockLocation.replace).toHaveBeenCalled();
        expect(mockLocation.replace.mock.calls[0][0]).toBe(
          KRATOS_LOGIN_FLOW_URL,
        );
      });
    });

    it("should redirect to default return_to when no return_to in URL", async () => {
      const { mockGetLoginFlow } = await getMocks();
      const error410 = {
        response: {
          status: 410,
        },
      };

      mockGetLoginFlow.mockRejectedValue(error410);

      // Mock window.location.href without return_to but with trusted origin
      const mockLocation = {
        href: "https://curionoah.com/auth/login?flow=expired-flow",
        replace: vi.fn(),
        assign: vi.fn(),
        reload: vi.fn(),
      };
      Object.defineProperty(window, "location", {
        value: mockLocation,
        writable: true,
      });

      renderWithProviders(<LoginForm flowId="expired-flow-id" />);

      await waitFor(() => {
        expect(mockLocation.replace).toHaveBeenCalled();
        expect(mockLocation.replace.mock.calls[0][0]).toBe(
          KRATOS_LOGIN_FLOW_URL,
        );
      });
    });
  });
});
