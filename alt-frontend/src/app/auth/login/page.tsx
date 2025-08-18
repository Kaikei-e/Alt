'use client'

import { useEffect, useState, useRef, Suspense } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Box, VStack, Text, Flex, Input, Button, Spinner } from '@chakra-ui/react'

interface LoginFlowNode {
  type: string
  group: string
  attributes: {
    name: string
    type: string
    required: boolean
    value?: string
  }
  messages?: Array<{
    text: string
    type: string
  }>
}

interface LoginFlow {
  id: string
  ui: {
    nodes: LoginFlowNode[]
    messages?: Array<{
      text: string
      type: string
    }>
  }
}

function LoginPageComponent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [flow, setFlow] = useState<LoginFlow | null>(null)
  const [formData, setFormData] = useState<Record<string, string>>({})
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [sessionChecked, setSessionChecked] = useState(false)
  const initiatedRef = useRef(false)

  const KRATOS_PUBLIC = "https://id.curionoah.com";

  // Get flow ID from query parameter or return URL
  const flowId = searchParams.get('flow')
  const returnUrl = searchParams.get('return_to') || '/'
  const refresh = searchParams.get('refresh') === 'true'

  // A. まずセッション確認によるスキップチェック
  useEffect(() => {
    const checkExistingSession = async () => {
      try {
        setIsLoading(true)

        // 再認証要求（refresh=true）の場合はセッションチェックをスキップ
        if (refresh) {
          setSessionChecked(true)
          setIsLoading(false)
          return
        }

        // whoami エンドポイントでセッション確認
        const response = await fetch("https://id.curionoah.com/sessions/whoami", {
          method: 'GET',
          credentials: 'include',
          headers: {
            'Accept': 'application/json',
          },
        })

        if (response.ok) {
          // 既にログイン済み - ホームへリダイレクト
          router.push(returnUrl)
          return
        }

        // セッションなし - ログインフロー続行
        setSessionChecked(true)

      } catch (err) {
        // エラーでもログインフローを続行
        console.warn('Session check failed, continuing with login flow:', err)
        setSessionChecked(true)
      } finally {
        setIsLoading(false)
      }
    }

    checkExistingSession()
  }, [router, returnUrl, refresh])

  useEffect(() => {
    if (!sessionChecked) return
    
    // Hydration前に空になるのを避ける: 直接location.searchを見る
    const hasFlow = typeof window !== 'undefined' && new URLSearchParams(window.location.search).has('flow')
    
    if (flowId || hasFlow) {
      // flowIdがあるか、直接URLにflowパラメータがある場合
      const actualFlowId = flowId || (typeof window !== 'undefined' ? new URLSearchParams(window.location.search).get('flow') : null)
      if (actualFlowId) {
        fetchFlow(actualFlowId)
      }
      return
    }
    
    // 一度だけ飛ばす（再レンダーでも再実行しない）
    if (!initiatedRef.current) {
      initiatedRef.current = true
      initiateLoginFlow()
    }
  }, [flowId, sessionChecked, refresh])

  const initiateLoginFlow = async () => {
    try {
      setIsLoading(true)
      setError(null)

      // B. 再認証時は ?refresh=true を付加
      const loginUrl = refresh
        ? "https://id.curionoah.com/self-service/login/browser?refresh=true"
        : "https://id.curionoah.com/self-service/login/browser"

      // 恒久対応: Kratos認証フロー初期化
      // ブラウザ方式でKratosに直接リダイレクトし、HTMLフローを開始
      window.location.href = loginUrl
      return

    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to initiate login')
    } finally {
      setIsLoading(false)
    }
  }

  const fetchFlow = async (id: string) => {
    try {
      setIsLoading(true)
      setError(null)

      const response = await fetch(`${KRATOS_PUBLIC}/self-service/login/flows?id=${id}`, {
        method: 'GET',
        credentials: 'include',
        headers: {
          'Accept': 'application/json',
        },
      })

      if (!response.ok) {
        if (response.status === 404 || response.status === 410) {
          // Flow expired or not found, initiate new flow
          initiateLoginFlow()
          return
        }
        throw new Error(`Failed to fetch login flow: ${response.status}`)
      }

      const flowData = await response.json()
      setFlow(flowData)

    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch login flow')
    } finally {
      setIsLoading(false)
    }
  }

  const handleInputChange = (name: string, value: string) => {
    setFormData(prev => ({
      ...prev,
      [name]: value
    }))
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!flow) {
      setError('No active login flow')
      return
    }

    try {
      setIsLoading(true)
      setError(null)

      const response = await fetch(`${KRATOS_PUBLIC}/self-service/login?flow=${flow.id}`, {
        method: 'POST',
        credentials: 'include',
        headers: {
          'Content-Type': 'application/json',
          'Accept': 'application/json',
        },
        body: JSON.stringify({
          method: 'password',
          ...formData,
          // 追加：CSRF
          csrf_token: flow.ui.nodes.find(n => n.attributes?.name === 'csrf_token')?.attributes?.value
        })
      })

      if (response.status === 303) {
        // Successful login, follow redirect
        const location = response.headers.get('Location')
        if (location) {
          window.location.href = location
        } else {
          // Fallback to return URL
          router.push(returnUrl)
        }
        return
      }

      if (response.status === 422) {
        // Validation errors, update flow
        const updatedFlow = await response.json()
        setFlow(updatedFlow)
        return
      }

      if (!response.ok) {
        throw new Error(`Login failed: ${response.status}`)
      }

      // Successful login
      router.push(returnUrl)

    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed')
    } finally {
      setIsLoading(false)
    }
  }

  const renderFormField = (node: LoginFlowNode) => {
    if (node.type !== 'input') return null;        // hidden も通す
    if (!['password', 'default'].includes(node.group)) return null; // csrf は default グループ

    const { name, type, required } = node.attributes
    const value = formData[name] || node.attributes.value || ''
    const messages = node.messages || []

    return (
      <Box key={name} w="full">
        <Input
          name={name}
          type={type}
          value={value}
          onChange={(e) => handleInputChange(name, e.target.value)}
          required={required}
          placeholder={name === 'identifier' ? 'Email' : 'Password'}
          bg="var(--alt-glass)"
          border="1px solid"
          borderColor="var(--alt-glass-border)"
          color="var(--text-primary)"
          _placeholder={{ color: 'var(--text-muted)' }}
          _focus={{
            borderColor: 'var(--alt-primary)',
            boxShadow: '0 0 0 1px var(--alt-primary)',
          }}
        />
        {messages.map((message, idx) => (
          <Text
            key={idx}
            fontSize="sm"
            color={message.type === 'error' ? 'red.400' : 'var(--text-muted)'}
            mt={1}
          >
            {message.text}
          </Text>
        ))}
      </Box>
    )
  }

  if (isLoading && (!flow || !sessionChecked)) {
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
            {!sessionChecked ? 'セッション確認中...' : 'ログインフローを準備中...'}
          </Text>
        </VStack>
      </Flex>
    )
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
              ログイン
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
              <Box p={3} bg="red.100" borderRadius="md" border="1px solid" borderColor="red.300" mb={4}>
                <Text fontSize="sm" color="red.700">{error}</Text>
              </Box>
            )}

            {flow && (
              <form onSubmit={handleSubmit}>
                <VStack gap={4}>
                  {flow.ui.messages?.map((message, idx) => (
                    <Box
                      key={idx}
                      p={3}
                      bg={message.type === 'error' ? 'red.100' : 'blue.100'}
                      borderRadius="md"
                      border="1px solid"
                      borderColor={message.type === 'error' ? 'red.300' : 'blue.300'}
                    >
                      <Text fontSize="sm" color={message.type === 'error' ? 'red.700' : 'blue.700'}>
                        {message.text}
                      </Text>
                    </Box>
                  ))}

                  {flow.ui.nodes.map(renderFormField)}

                  <Button
                    type="submit"
                    w="full"
                    bg="var(--alt-primary)"
                    color="white"
                    size="lg"
                    disabled={isLoading}
                    _hover={{ bg: 'var(--alt-primary-hover)' }}
                    _active={{ bg: 'var(--alt-primary-active)' }}
                  >
                    {isLoading ? 'ログイン中...' : 'ログイン'}
                  </Button>
                </VStack>
              </form>
            )}
          </Box>

          <Box textAlign="center">
            <Text
              fontSize="sm"
              color="var(--text-muted)"
              fontFamily="body"
            >
              アカウントをお持ちでない方は{' '}
              <Box
                as="button"
                color="var(--alt-primary)"
                textDecoration="underline"
                onClick={() => window.location.href = "https://id.curionoah.com/self-service/registration/browser"}
              >
                新規登録
              </Box>
            </Text>
          </Box>
        </VStack>
      </Flex>
    </Box>
  )
}

export default function LoginPage() {
  return (
    <Suspense fallback={
      <Flex
        minH="100vh"
        align="center"
        justify="center"
        bg="var(--alt-glass-bg)"
      >
        <VStack gap={4}>
          <Spinner size="lg" color="var(--alt-primary)" />
          <Text color="var(--text-primary)" fontFamily="body">
            ログインページを準備中...
          </Text>
        </VStack>
      </Flex>
    }>
      <LoginPageComponent />
    </Suspense>
  )
}