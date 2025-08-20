'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
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

interface LoginClientProps {
  flowId: string
  returnUrl: string
}

export default function LoginClient({ flowId, returnUrl }: LoginClientProps) {
  const router = useRouter()
  const [flow, setFlow] = useState<LoginFlow | null>(null)
  const [formData, setFormData] = useState<Record<string, string>>({})
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  
  // TODO.md: 三値判定 null=不明, false=未ログイン, true=ログイン済み
  const [session, setSession] = useState<null | boolean>(null)

  const KRATOS_PUBLIC = process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL!
  if (!KRATOS_PUBLIC) throw new Error('NEXT_PUBLIC_KRATOS_PUBLIC_URL missing')

  useEffect(() => {
    if (!flowId) return
    fetchFlow(flowId)
  }, [flowId])

  // TODO.md: 三値判定のセッションチェック
  useEffect(() => {
    let abort = false;
    (async () => {
      try {
        const r = await fetch('/api/auth/validate', { 
          credentials: 'include', 
          cache: 'no-store' 
        });
        if (abort) return;
        setSession(r.ok); // ok=true=ログイン済 / それ以外は false
      } catch { 
        if (!abort) setSession(false); 
      }
    })();
    return () => { abort = true; };
  }, []);

  // TODO.md: ログイン済みの場合のみ router.replace 実行
  useEffect(() => {
    if (session === true) {
      router.replace(returnUrl);
    }
  }, [session, router, returnUrl]);

  // Session cleanup helper for redirect loop prevention
  const cleanupSession = async () => {
    try {
      await fetch('/api/auth/cleanup', {
        method: 'POST',
        credentials: 'include',
      })
      console.log('Session cleanup completed')
    } catch (error) {
      console.warn('Session cleanup failed:', error)
    }
  }

  const fetchFlow = async (id: string) => {
    try {
      setIsLoading(true)
      setError(null)

      const response = await fetch(
        `${KRATOS_PUBLIC}/self-service/login/flows?id=${id}`,
        {
          credentials: 'include',
          headers: { Accept: 'application/json' },
          cache: 'no-store', // ← キャッシュ回避
        }
      )

      if (!response.ok) {
        if (response.status === 410) { 
          // Flow expired - clean up stale sessions before redirect
          await cleanupSession()
          window.location.href = '/auth/login'; return
        }
        if (response.status === 404) {
          // Flow not found - could indicate session issues
          await cleanupSession()
          window.location.href = '/auth/login'; return
        }
        setError(`fetch flow failed: ${response.status}`); return
      }

      const flowData = await response.json()
      setFlow(flowData)
      return flowData

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

    // フローが未ロードならここで取得し、ローカル変数 f を使う
    let f = flow
    if (!f) {
      f = await fetchFlow(flowId)
    }
    const csrf = f?.ui?.nodes?.find(
      (n: any) => n.attributes?.name === 'csrf_token'
    )?.attributes?.value
    if (!csrf) {
      // CSRFが取れない＝期限切れorキャッシュ汚染。セッション清理して再初期化
      await cleanupSession()
      window.location.href = '/auth/login'
      return
    }
    
    try {
      setIsLoading(true)
      setError(null)

      const resp = await fetch(`${KRATOS_PUBLIC}/self-service/login?flow=${flowId}`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
        body: JSON.stringify({
          method: 'password',
          csrf_token: csrf,
          ...formData
        }),
      })
      
      if (resp.status === 422) { 
        setFlow(await resp.json()); return // バリデーションエラー
      }
      if (!resp.ok) { 
        setError(`login failed: ${resp.status}`); return 
      }
      router.push(returnUrl)

    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed')
    } finally {
      setIsLoading(false)
    }
  }

  const renderFormField = (node: LoginFlowNode) => {
    if (node.type !== 'input') return null
    if (!['password', 'default'].includes(node.group)) return null

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
            ログインフローを準備中...
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
              <a
                href={`${KRATOS_PUBLIC}/self-service/registration/browser?return_to=${encodeURIComponent(returnUrl)}`}
                style={{
                  color: 'var(--alt-primary)',
                  textDecoration: 'underline'
                }}
              >
                新規登録
              </a>
            </Text>
          </Box>
        </VStack>
      </Flex>
    </Box>
  )
}