"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import {
  Box,
  VStack,
  Text,
  Flex,
  Input,
  Button,
  Spinner,
} from "@chakra-ui/react";
import {
  Configuration,
  FrontendApi,
  RegistrationFlow,
  UiNode,
  UiNodeInputAttributes,
  UpdateRegistrationFlowBody,
} from "@ory/client";

// URL validation helper to prevent open redirects
const isValidReturnUrl = (url: string): boolean => {
  try {
    const parsedUrl = new URL(url);
    const appOrigin = process.env.NEXT_PUBLIC_APP_ORIGIN;
    const idpOrigin = process.env.NEXT_PUBLIC_IDP_ORIGIN;

    // Only allow same-origin or trusted IDP origin redirects
    return parsedUrl.origin === appOrigin || parsedUrl.origin === idpOrigin;
  } catch {
    return false;
  }
};

// Safe redirect helper
const safeRedirect = (url: string) => {
  if (isValidReturnUrl(url)) {
    window.location.href = url;
  } else {
    // Fallback to default safe URL
    window.location.href = process.env.NEXT_PUBLIC_RETURN_TO_DEFAULT || "/";
  }
};

const frontend = new FrontendApi(
  new Configuration({ basePath: process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL, baseOptions: { credentials: "include" } }),
);

interface RegisterClientProps {
  flowId: string;
  returnUrl: string;
}

export default function RegisterClient({
  flowId,
  returnUrl,
}: RegisterClientProps) {
  const router = useRouter();
  const [flow, setFlow] = useState<RegistrationFlow | null>(null);
  const [formData, setFormData] = useState<Record<string, string>>({});
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!flowId) return;
    fetchFlow(flowId);
  }, [flowId]);

  const fetchFlow = async (id: string) => {
    try {
      setIsLoading(true);
      setError(null);

      const { data } = await frontend.getRegistrationFlow({ id });
      setFlow(data);
      const initialValues: Record<string, string> = {};
      data.ui?.nodes?.forEach((node) => {
        if (node.type !== "input" || !node.attributes) return;
        const attrs = node.attributes as UiNodeInputAttributes;
        if (!attrs?.name) return;
        if (typeof attrs.value === "string") {
          initialValues[attrs.name] = attrs.value;
        }
      });
      setFormData(initialValues);
    } catch (err) {
      const error = err as {
        response?: { status?: number };
        status?: number;
      };
      const status = error.response?.status ?? error.status;
      if (status === 410 || status === 403) {
        safeRedirect("/register");
        return;
      }
      setError(
        err instanceof Error
          ? err.message
          : "Failed to fetch registration flow",
      );
    } finally {
      setIsLoading(false);
    }
  };

  const handleInputChange = (name: string, value: string) => {
    setFormData((prev) => ({
      ...prev,
      [name]: value,
    }));
  };

  const isRegistrationFlow = (value: unknown): value is RegistrationFlow => {
    if (!value || typeof value !== "object") return false;
    return "id" in value && "ui" in value;
  };

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    if (!flow) {
      setError("No active registration flow");
      return;
    }

    try {
      setIsLoading(true);
      setError(null);

      const formEntries = Object.fromEntries(
        new FormData(e.currentTarget).entries(),
      ) as Record<string, string>;

      const methodValue =
        (formEntries.method as UpdateRegistrationFlowBody["method"]) ||
        (getNodeValue(flow.ui.nodes, "method") as UpdateRegistrationFlowBody["method"]) ||
        "password";

      const payload = {
        ...formEntries,
        method: methodValue,
      } as UpdateRegistrationFlowBody;

      const { data } = await frontend.updateRegistrationFlow({
        flow: flow.id,
        updateRegistrationFlowBody: payload,
      });

      if (isRegistrationFlow(data)) {
        if (data.ui?.messages?.length) {
          setFlow(data);
          return;
        }
      }

      router.push(returnUrl);
    } catch (err) {
      const error = err as {
        response?: { status?: number; data?: RegistrationFlow };
        status?: number;
        message?: string;
      };
      const status = error.response?.status ?? error.status;
      if (status === 403) {
        safeRedirect("/register");
        return;
      }
      if (status === 422 && error.response?.data) {
        setFlow(error.response.data as RegistrationFlow);
        return;
      }
      setError(error?.message ?? "Registration failed");
    } finally {
      setIsLoading(false);
    }
  };

  const getNodeValue = (nodes: UiNode[], name: string): string => {
    const node = nodes.find((n) => {
      if (!("attributes" in n) || !n.attributes) return false;
      const attrs = n.attributes as UiNodeInputAttributes;
      return attrs.name === name;
    });
    if (!node || !node.attributes) return "";
    const attrs = node.attributes as UiNodeInputAttributes;
    if (typeof attrs.value === "string") return attrs.value;
    if (typeof attrs.value === "number") return String(attrs.value);
    return "";
  };

  const renderFormField = (node: UiNode) => {
    if (node.type !== "input") return null;
    if (!["password", "default"].includes(node.group)) {
      return null;
    }

    const attrs = node.attributes as UiNodeInputAttributes;
    if (!attrs?.name || !attrs.type) {
      return null;
    }

    const { name, required } = attrs;
    const inputType =
      name === "password" && attrs.type !== "password"
        ? "password"
        : attrs.type;
    const defaultValue = typeof attrs.value === "string" ? attrs.value : "";
    const value = formData[name] ?? defaultValue;
    const messages = node.messages || [];

    let placeholder = name;
    if (name === "traits.email") placeholder = "Email";
    else if (name === "password") placeholder = "Password";
    else if (name === "traits.name") placeholder = "Name";

    return (
      <Box key={name} w="full">
        <Input
          name={name}
          type={inputType}
          value={value}
          onChange={(e) => handleInputChange(name, e.target.value)}
          required={required}
          placeholder={placeholder}
          bg="var(--alt-glass)"
          border="1px solid"
          borderColor="var(--alt-glass-border)"
          color="var(--text-primary)"
          _placeholder={{ color: "var(--text-muted)" }}
          _focus={{
            borderColor: "var(--alt-primary)",
            boxShadow: "0 0 0 1px var(--alt-primary)",
          }}
        />
        {messages.map((message, idx) => (
          <Text
            key={idx}
            fontSize="sm"
            color={message.type === "error" ? "red.400" : "var(--text-muted)"}
            mt={1}
          >
            {message.text}
          </Text>
        ))}
      </Box>
    );
  };

  if (isLoading && !flow) {
    return (
      <Flex
        minH="100vh"
        align="center"
        justify="center"
        bg="var(--alt-glass-bg)"
      >
        <VStack gap={4}>
          <Spinner size="lg" color="var(--alt-primary)" />
          <Text color="var(--text-primary)" fontFamily="body">
            登録フローを準備中...
          </Text>
        </VStack>
      </Flex>
    );
  }

  return (
    <Box
      minH="100vh"
      bg="var(--alt-glass-bg)"
      bgImage="radial-gradient(circle at 25% 25%, var(--alt-glass) 0%, transparent 70%), radial-gradient(circle at 75% 75%, var(--alt-primary-alpha) 0%, transparent 70%)"
      position="relative"
      overflow="hidden"
    >
      <Box
        position="absolute"
        top="0"
        left="0"
        right="0"
        bottom="0"
        bgImage="url('data:image/svg+xml;charset=utf-8,%3Csvg width=%2760%27 height=%2760%27 viewBox=%270 0 60 60%27 xmlns=%27http://www.w3.org/2000/svg%27%3E%3Cg fill=%27none%27 fill-rule=%27evenodd%27%3E%3Cg fill=%27%23ffffff%27 fill-opacity=%270.03%27%3E%3Ccircle cx=%2730%27 cy=%2730%27 r=%271%27/%3E%3C/g%3E%3C/svg%3E')"
        pointerEvents="none"
      />

      <Flex
        minH="100vh"
        align="center"
        justify="center"
        p={4}
        position="relative"
        zIndex={1}
      >
        <VStack gap={8} w="full" maxW="400px">
          <VStack gap={4} textAlign="center">
            <Text
              fontSize="2xl"
              fontWeight="bold"
              fontFamily="heading"
              color="var(--alt-primary)"
              textShadow="0 2px 4px rgba(0,0,0,0.1)"
            >
              Alt
            </Text>
            <Text
              fontSize="lg"
              fontWeight="semibold"
              fontFamily="heading"
              color="var(--text-primary)"
            >
              新規登録
            </Text>
          </VStack>

          <Box
            w="full"
            p={6}
            bg="var(--alt-glass)"
            border="1px solid"
            borderColor="var(--alt-glass-border)"
            borderRadius="lg"
            backdropFilter="blur(12px)"
          >
            {error && (
              <Box
                p={3}
                bg="red.100"
                borderRadius="md"
                border="1px solid"
                borderColor="red.300"
                mb={4}
              >
                <Text fontSize="sm" color="red.700">
                  {error}
                </Text>
              </Box>
            )}

            {flow && (
              <form onSubmit={handleSubmit}>
                <VStack gap={4}>
                  {flow.ui.messages?.map((message, idx) => (
                    <Box
                      key={idx}
                      p={3}
                      bg={message.type === "error" ? "red.100" : "blue.100"}
                      borderRadius="md"
                      border="1px solid"
                      borderColor={
                        message.type === "error" ? "red.300" : "blue.300"
                      }
                    >
                      <Text
                        fontSize="sm"
                        color={
                          message.type === "error" ? "red.700" : "blue.700"
                        }
                      >
                        {message.text}
                      </Text>
                    </Box>
                  ))}

                  <input
                    type="hidden"
                    name="method"
                    value={getNodeValue(flow.ui.nodes, "method") || "password"}
                  />
                  <input
                    type="hidden"
                    name="csrf_token"
                    value={getNodeValue(flow.ui.nodes, "csrf_token")}
                  />

                  {flow.ui.nodes.map(renderFormField)}

                  <Button
                    type="submit"
                    w="full"
                    bg="var(--alt-primary)"
                    color="white"
                    size="lg"
                    disabled={isLoading}
                    _hover={{ bg: "var(--alt-primary-hover)" }}
                    _active={{ bg: "var(--alt-primary-active)" }}
                  >
                    {isLoading ? "登録中..." : "新規登録"}
                  </Button>
                </VStack>
              </form>
            )}
          </Box>

          <Box textAlign="center">
            <Text fontSize="sm" color="var(--text-muted)" fontFamily="body">
              既にアカウントをお持ちの方は{" "}
              <Box
                as="button"
                color="var(--alt-primary)"
                textDecoration="underline"
                onClick={() => {
                  const currentUrl =
                    typeof window !== "undefined" ? window.location.href : "/";
                  const returnUrl = encodeURIComponent(currentUrl);
                  const loginUrl = `/auth/login?return_to=${returnUrl}`;
                  safeRedirect(loginUrl);
                }}
              >
                ログイン
              </Box>
            </Text>
          </Box>
        </VStack>
      </Flex>
    </Box>
  );
}
