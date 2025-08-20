'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { Box, VStack, Text, Flex, Input, Button, Spinner } from '@chakra-ui/react'

interface RegistrationFlowNode {
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

interface RegistrationFlow {
  id: string
  ui: {
    nodes: RegistrationFlowNode[]
    messages?: Array<{
      text: string
      type: string
    }>
  }
}

interface RegisterClientProps {
  flowId: string
  returnUrl: string
}

export default function RegisterClient({ flowId, returnUrl }: RegisterClientProps) {
  const router = useRouter()
  const [flow, setFlow] = useState<RegistrationFlow | null>(null)
  const [formData, setFormData] = useState<Record<string, string>>({})
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const KRATOS_PUBLIC = process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL!
  if (!KRATOS_PUBLIC) throw new Error('NEXT_PUBLIC_KRATOS_PUBLIC_URL missing')

  useEffect(() => {
    if (!flowId) return
    fetchFlow(flowId)
  }, [flowId])

  const fetchFlow = async (id: string) => {
    try {
      setIsLoading(true)
      setError(null)

      const response = await fetch(`${KRATOS_PUBLIC}/self-service/registration/flows?id=${id}`, {
        method: 'GET',
        credentials: 'include',
        headers: {
          'Accept': 'application/json',
        },
      })

      if (!response.ok) {
        if (response.status === 410) {        // ← 期限切れだけ"新規フロー"
          window.location.href = '/register'
          return
        }
        // 404・403・429 等はここで可視化（無限再初期化しない）
        const msg = `Failed to fetch registration flow: ${response.status}`
        setError(msg); setIsLoading(false); return
      }

      const flowData = await response.json()
      setFlow(flowData)

    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch registration flow')
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
      setError('No active registration flow')
      return
    }

    try {
      setIsLoading(true)
      setError(null)

      const csrf = flow.ui.nodes.find(n => n.attributes?.name === 'csrf_token')?.attributes?.value

      const response = await fetch(`${KRATOS_PUBLIC}/self-service/registration?flow=${flow.id}`, {
        method: 'POST',
        credentials: 'include',
        headers: {
          'Content-Type': 'application/json',
          'Accept': 'application/json',
        },
        body: JSON.stringify({
          method: 'password',
          ...formData,
          csrf_token: csrf
        })
      })

      // JSONモード：200(OK)/422(検証エラー) を扱う
      if (response.status === 422) {
        const updatedFlow = await response.json()
        setFlow(updatedFlow)
        return
      }

      if (!response.ok) {
        throw new Error(`Registration failed: ${response.status}`)
      }

      router.push(returnUrl)

    } catch (err) {
      setError(err instanceof Error ? err.message : 'Registration failed')
    } finally {
      setIsLoading(false)
    }
  }

  const renderFormField = (node: RegistrationFlowNode) => {
    if (node.type !== 'input') return null
    if (!['password','default'].includes(node.group)) {
      return null
    }

    const { name, type, required } = node.attributes
    const value = formData[name] || node.attributes.value || ''
    const messages = node.messages || []

    let placeholder = name
    if (name === 'traits.email') placeholder = 'Email'
    else if (name === 'password') placeholder = 'Password'
    else if (name === 'traits.name') placeholder = 'Name'

    return (
      <Box key={name} w="full">
        <Input
          name={name}
          type={type}
          value={value}
          onChange={(e) => handleInputChange(name, e.target.value)}
          required={required}
          placeholder={placeholder}
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
            登録フローを準備中...
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
                    {isLoading ? '登録中...' : '新規登録'}
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
              既にアカウントをお持ちの方は{' '}
              <Box
                as="button"
                color="var(--alt-primary)"
                textDecoration="underline"
                onClick={() => window.location.href = "/login"}
              >
                ログイン
              </Box>
            </Text>
          </Box>
        </VStack>
      </Flex>
    </Box>
  )
}
