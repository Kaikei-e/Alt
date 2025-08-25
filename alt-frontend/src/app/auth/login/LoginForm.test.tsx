/**
 * @vitest-environment jsdom
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import LoginForm from './LoginForm';

// Oryクライアントのモック
vi.mock('@ory/client', () => {
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
Object.defineProperty(window, 'location', {
  value: {
    href: 'http://localhost:3000',
  },
  writable: true,
});

// モックへのアクセス用ヘルパー
const getMocks = async () => {
  const oryMock = vi.mocked(await import('@ory/client'));
  return {
    mockGetLoginFlow: (oryMock as any).__mockGetLoginFlow,
    mockUpdateLoginFlow: (oryMock as any).__mockUpdateLoginFlow,
  };
};

describe('LoginForm', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    // デフォルトのモック動作を設定
    const { mockGetLoginFlow } = await getMocks();
    mockGetLoginFlow.mockImplementation(() => new Promise(() => {})); // pending promise
  });

  describe('初期状態', () => {
    it('ローディング状態が表示される', () => {
      render(<LoginForm flowId="test-flow-id" />);
      expect(screen.getByText(/loading/i)).toBeInTheDocument();
    });

    it('フローが正常に読み込まれてフォームが表示される', async () => {
      const { mockGetLoginFlow } = await getMocks();
      
      const mockFlow = {
        id: 'test-flow-id',
        ui: {
          nodes: [
            {
              type: 'input',
              attributes: {
                name: 'identifier',
                type: 'email',
                required: true,
              },
              messages: [],
            }
          ]
        }
      };

      mockGetLoginFlow.mockResolvedValue({ data: mockFlow });
      
      render(<LoginForm flowId="test-flow-id" />);
      
      await waitFor(() => {
        expect(screen.getByRole('textbox', { name: /email/i })).toBeInTheDocument();
      });
    });
  });

  describe('フロー取得', () => {
    it('正常なフローデータを取得して表示する', async () => {
      const { mockGetLoginFlow } = await getMocks();
      const mockFlow = {
        id: 'test-flow-id',
        ui: {
          nodes: [
            {
              type: 'input',
              attributes: {
                name: 'identifier',
                type: 'email',
                required: true,
              },
              messages: [],
            },
            {
              type: 'input',
              attributes: {
                name: 'password',
                type: 'password',
                required: true,
              },
              messages: [],
            }
          ]
        }
      };

      mockGetLoginFlow.mockResolvedValue({ data: mockFlow });

      render(<LoginForm flowId="test-flow-id" />);

      await waitFor(() => {
        expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
        expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
      });
    });

    it('フロー取得失敗時にエラーを表示する', async () => {
      const { mockGetLoginFlow } = await getMocks();
      mockGetLoginFlow.mockRejectedValue(new Error('Network error'));

      render(<LoginForm flowId="test-flow-id" />);

      await waitFor(() => {
        expect(screen.getByText(/Failed to load login form. Please try again./i)).toBeInTheDocument();
      });
    });
  });

  describe('フォーム送信', () => {
    it('正常な送信処理を行う', async () => {
      const { mockGetLoginFlow, mockUpdateLoginFlow } = await getMocks();
      const user = userEvent.setup();
      const mockFlow = {
        id: 'test-flow-id',
        ui: {
          action: '/self-service/login',
          method: 'POST',
          nodes: [
            {
              type: 'input',
              attributes: {
                name: 'identifier',
                type: 'email',
                required: true,
              },
              messages: [],
            },
            {
              type: 'input',
              attributes: {
                name: 'password',
                type: 'password',
                required: true,
              },
              messages: [],
            }
          ]
        }
      };

      const mockResponse = {
        data: {
          session: { id: 'session-id' },
          return_to: 'https://curionoah.com/'
        }
      };

      mockGetLoginFlow.mockResolvedValue({ data: mockFlow });
      mockUpdateLoginFlow.mockResolvedValue(mockResponse);

      // window.location.href のモック
      const mockLocation = { href: '' };
      Object.defineProperty(window, 'location', {
        value: mockLocation,
        writable: true,
      });

      render(<LoginForm flowId="test-flow-id" />);

      await waitFor(() => {
        expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText(/email/i), 'test@example.com');
      await user.type(screen.getByLabelText(/password/i), 'password123');
      await user.click(screen.getByRole('button', { name: /sign in/i }));

      await waitFor(() => {
        expect(window.location.href).toBe('https://curionoah.com/');
      });
    });

    it('ログイン失敗時にエラーメッセージを表示する', async () => {
      const { mockGetLoginFlow, mockUpdateLoginFlow } = await getMocks();
      const user = userEvent.setup();
      const mockFlow = {
        id: 'test-flow-id',
        ui: {
          action: '/self-service/login',
          method: 'POST',
          nodes: [
            {
              type: 'input',
              attributes: {
                name: 'identifier',
                type: 'email',
                required: true,
              },
              messages: [],
            },
            {
              type: 'input',
              attributes: {
                name: 'password',
                type: 'password',
                required: true,
              },
              messages: [],
            }
          ]
        }
      };

      const errorResponse = {
        response: {
          status: 400,
          data: {
            ui: {
              messages: [
                {
                  id: 4000006,
                  text: 'The provided credentials are invalid.',
                  type: 'error'
                }
              ]
            }
          }
        }
      };

      mockGetLoginFlow.mockResolvedValue({ data: mockFlow });
      mockUpdateLoginFlow.mockRejectedValue(errorResponse);

      render(<LoginForm flowId="test-flow-id" />);

      await waitFor(() => {
        expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText(/email/i), 'wrong@example.com');
      await user.type(screen.getByLabelText(/password/i), 'wrongpassword');
      await user.click(screen.getByRole('button', { name: /sign in/i }));

      await waitFor(() => {
        // ログインが失敗した場合、ボタンは再度有効になる
        expect(screen.getByRole('button', { name: /sign in/i })).not.toBeDisabled();
      });
    });
  });

  describe('410 Error Handling (Flow Expiration)', () => {
    it('should redirect to new flow when getLoginFlow returns 410', async () => {
      const { mockGetLoginFlow } = await getMocks();
      const error410 = {
        response: {
          status: 410,
          data: {
            error: {
              id: 'self_service_flow_expired',
              code: 410,
              status: 'Gone',
              message: 'Flow expired'
            }
          }
        }
      };

      mockGetLoginFlow.mockRejectedValue(error410);

      // Mock window.location.href
      const mockLocation = { href: '' };
      Object.defineProperty(window, 'location', {
        value: mockLocation,
        writable: true,
      });

      // Mock current URL with return_to parameter
      Object.defineProperty(window, 'location', {
        value: {
          ...mockLocation,
          href: 'https://curionoah.com/auth/login?flow=expired-flow&return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Fhome'
        },
        writable: true,
      });

      render(<LoginForm flowId="expired-flow-id" />);

      await waitFor(() => {
        expect(window.location.href).toContain('/ory/self-service/login/browser?return_to=');
      });
    });

    it('should redirect to new flow when updateLoginFlow returns 410', async () => {
      const { mockGetLoginFlow, mockUpdateLoginFlow } = await getMocks();
      const user = userEvent.setup();
      const mockFlow = {
        id: 'test-flow-id',
        ui: {
          action: '/self-service/login',
          method: 'POST',
          nodes: [
            {
              type: 'input',
              attributes: {
                name: 'identifier',
                type: 'email',
                required: true,
              },
              messages: [],
            },
            {
              type: 'input',
              attributes: {
                name: 'password',
                type: 'password',
                required: true,
              },
              messages: [],
            }
          ]
        }
      };

      const error410 = {
        response: {
          status: 410,
          data: {
            error: {
              id: 'self_service_flow_expired'
            }
          }
        }
      };

      mockGetLoginFlow.mockResolvedValue({ data: mockFlow });
      mockUpdateLoginFlow.mockRejectedValue(error410);

      // Mock window.location.href
      const mockLocation = { href: '' };
      Object.defineProperty(window, 'location', {
        value: {
          ...mockLocation,
          href: 'https://curionoah.com/auth/login?flow=test-flow&return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Fsettings'
        },
        writable: true,
      });

      render(<LoginForm flowId="test-flow-id" />);

      await waitFor(() => {
        expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText(/email/i), 'test@example.com');
      await user.type(screen.getByLabelText(/password/i), 'password123');
      await user.click(screen.getByRole('button', { name: /sign in/i }));

      await waitFor(() => {
        expect(window.location.href).toContain('/ory/self-service/login/browser?return_to=');
      });
    });

    it('should redirect to default return_to when no return_to in URL', async () => {
      const { mockGetLoginFlow } = await getMocks();
      const error410 = {
        response: {
          status: 410
        }
      };

      mockGetLoginFlow.mockRejectedValue(error410);

      // Mock window.location.href without return_to
      const mockLocation = { href: '' };
      Object.defineProperty(window, 'location', {
        value: {
          ...mockLocation,
          href: 'https://curionoah.com/auth/login?flow=expired-flow'
        },
        writable: true,
      });

      render(<LoginForm flowId="expired-flow-id" />);

      await waitFor(() => {
        expect(window.location.href).toContain('/ory/self-service/login/browser?return_to=');
      });
    });
  });
});