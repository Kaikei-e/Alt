"use client";

import type { FormEvent, ReactNode } from "react";
import NextLink from "next/link";
import {
  Box,
  Button,
  Flex,
  Image,
  Input,
  Link as ChakraLink,
  Text,
} from "@chakra-ui/react";
import type {
  LoginFlow,
  RegistrationFlow,
  UiNode,
  UiNodeAnchorAttributes,
  UiNodeImageAttributes,
  UiNodeInputAttributes,
  UiNodeTextAttributes,
} from "@ory/client";

export interface OryFlowFormProps {
  flow: LoginFlow | RegistrationFlow | null;
  isSubmitting?: boolean;
  isLoading?: boolean;
  error?: string | null;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  emptyState?: React.ReactNode;
}

const renderMessages = (messages?: UiNode["messages"]) =>
  messages?.map((message, idx) => (
    <Text
      key={`message-${idx}-${message.text}`}
      fontSize="sm"
      color={message.type === "error" ? "red.400" : "var(--text-muted)"}
    >
      {message.text}
    </Text>
  ));

const renderTextNode = (node: UiNode) => {
  const attrs = node.attributes as UiNodeTextAttributes;
  const text = attrs?.text?.text;
  if (!text) return null;
  return (
    <Text fontSize="sm" color="var(--text-muted)">
      {text}
    </Text>
  );
};

const renderAnchorNode = (node: UiNode) => {
  const attrs = node.attributes as UiNodeAnchorAttributes;
  if (!attrs?.href) return null;
  return (
    <ChakraLink as={NextLink} href={attrs.href} color="var(--alt-primary)">
      {attrs.title?.text ?? attrs.href}
    </ChakraLink>
  );
};

const renderImageNode = (node: UiNode) => {
  const attrs = node.attributes as UiNodeImageAttributes;
  if (!attrs?.src) return null;
  return <Image src={attrs.src} alt="" />;
};

const renderInputNode = (
  node: UiNode,
  index: number,
  isSubmitting?: boolean,
) => {
  const attrs = node.attributes as UiNodeInputAttributes;

  if (!attrs?.name) {
    return null;
  }

  if (attrs.type === "hidden") {
    const value = typeof attrs.value === "string" ? attrs.value : "";
    return (
      <input
        key={`hidden-${index}-${attrs.name}`}
        type="hidden"
        name={attrs.name}
        value={value}
      />
    );
  }

  if (attrs.type === "submit" || attrs.type === "button") {
    const value = typeof attrs.value === "string" ? attrs.value : attrs.name;
    const handleClick = () => {
      if (typeof window === "undefined") return;
      const trigger = attrs.onclickTrigger;
      if (trigger) {
        const triggerFn = (window as unknown as Record<string, unknown>)[
          trigger
        ];
        if (typeof triggerFn === "function") {
          (triggerFn as () => void)();
        }
      }
    };

    return (
      <Button
        key={`button-${index}-${attrs.name}-${value}`}
        type={attrs.type === "submit" ? "submit" : "button"}
        name={attrs.name}
        value={value}
        onClick={handleClick}
        width="100%"
        bg="var(--alt-primary)"
        color="white"
        disabled={isSubmitting && attrs.type === "submit"}
        _hover={{ bg: "var(--alt-primary-hover)" }}
        _active={{ bg: "var(--alt-primary-active)" }}
      >
        {attrs.label?.text ?? node.meta?.label?.text ?? value}
      </Button>
    );
  }

  if (attrs.type === "checkbox") {
    const checked =
      attrs.value === true || attrs.value === "true" || attrs.value === "on";
    return (
      <Flex
        key={`checkbox-${index}-${attrs.name}`}
        align="center"
        gap={2}
        as="label"
        color="var(--text-muted)"
        fontSize="sm"
      >
        <input
          type="checkbox"
          name={attrs.name}
          value="true"
          defaultChecked={checked}
          disabled={attrs.disabled}
          style={{ accentColor: "var(--alt-primary)" }}
        />
        {attrs.label?.text ?? node.meta?.label?.text ?? attrs.name}
      </Flex>
    );
  }

  const defaultValue =
    typeof attrs.value === "string" ? attrs.value : undefined;
  const label = attrs.label?.text ?? node.meta?.label?.text ?? attrs.name;

  return (
    <Box key={`input-${index}-${attrs.name}`} width="100%">
      <Text fontSize="sm" color="var(--text-muted)" mb={1}>
        {label}
      </Text>
      <Input
        name={attrs.name}
        type={attrs.type === "11184809" ? "text" : attrs.type}
        placeholder={label}
        defaultValue={defaultValue}
        required={attrs.required}
        autoComplete={attrs.autocomplete ?? undefined}
        disabled={attrs.disabled}
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
      {renderMessages(node.messages)}
    </Box>
  );
};

const renderNode = (
  node: UiNode,
  index: number,
  isSubmitting?: boolean,
): ReactNode => {
  switch (node.type) {
    case "input":
      return renderInputNode(node, index, isSubmitting);
    case "text":
      return <Box key={`text-${index}`}>{renderTextNode(node)}</Box>;
    case "img":
      return <Box key={`image-${index}`}>{renderImageNode(node)}</Box>;
    case "a":
      return <Box key={`anchor-${index}`}>{renderAnchorNode(node)}</Box>;
    default:
      return null;
  }
};

export const OryFlowForm = ({
  flow,
  isSubmitting,
  isLoading,
  error,
  onSubmit,
  emptyState,
}: OryFlowFormProps) => {
  if (isLoading) {
    return (
      <Flex direction="column" align="center" gap={4} py={12}>
        <Text color="var(--text-muted)">フローを読み込んでいます…</Text>
      </Flex>
    );
  }

  if (!flow) {
    return <Box>{emptyState ?? <Text>フローが見つかりません。</Text>}</Box>;
  }

  const hiddenNodes = flow.ui.nodes.filter((node) => {
    if (node.type !== "input") return false;
    const attrs = node.attributes as UiNodeInputAttributes;
    return attrs.type === "hidden";
  });

  const visibleNodes = flow.ui.nodes.filter((node) => {
    if (node.type !== "input") return true;
    const attrs = node.attributes as UiNodeInputAttributes;
    return attrs.type !== "hidden";
  });

  return (
    <form
      action={flow.ui.action}
      method={flow.ui.method}
      onSubmit={onSubmit}
      noValidate
      style={{ width: "100%" }}
    >
      {hiddenNodes.map((node, index) =>
        renderInputNode(node, index, isSubmitting),
      )}

      <Flex direction="column" gap={4} width="100%">
        {error && (
          <Box
            p={3}
            border="1px solid"
            borderColor="red.300"
            bg="red.100"
            borderRadius="md"
          >
            <Text fontSize="sm" color="red.700">
              {error}
            </Text>
          </Box>
        )}

        {flow.ui.messages?.length ? (
          <Box>{renderMessages(flow.ui.messages)}</Box>
        ) : null}

        {visibleNodes.map((node, index) =>
          renderNode(node, index, isSubmitting),
        )}
      </Flex>
    </form>
  );
};
