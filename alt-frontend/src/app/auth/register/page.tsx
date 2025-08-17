'use client'

import { useEffect, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Box, VStack, Text, Flex, Input, Button, Alert, Spinner } from '@chakra-ui/react'

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

export default function RegisterPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [flow, setFlow] = useState<RegistrationFlow | null>(null)
  const [formData, setFormData] = useState<Record<string, string>>({})
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Get flow ID from query parameter or return URL
  const flowId = searchParams.get('flow')
  const returnUrl = searchParams.get('return_to') || '/'

  useEffect(() => {
    if (flowId) {
      // If we have a flow ID, fetch the flow data
      fetchFlow(flowId)
    } else {
      // If no flow ID, initiate a new registration flow
      initiateRegistrationFlow()
    }
  }, [flowId])

  const initiateRegistrationFlow = async () => {
    try {
      setIsLoading(true)
      setError(null)

      const response = await fetch('/self-service/registration/browser', {
        method: 'GET',
        credentials: 'include',
        headers: {
          'Accept': 'application/json',
        },
      })

      if (response.status === 303) {
        // Handle redirect to Kratos flow
        const location = response.headers.get('Location')
        if (location) {
          window.location.href = location
          return
        }
      }

      if (!response.ok) {
        throw new Error(`Failed to initiate registration flow: ${response.status}`)
      }

      const flowData = await response.json()
      setFlow(flowData)
      
      // Update URL with flow ID without page reload
      const newUrl = new URL(window.location.href)
      newUrl.searchParams.set('flow', flowData.id)
      window.history.replaceState({}, '', newUrl.toString())

    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to initiate registration')
    } finally {
      setIsLoading(false)
    }
  }

  const fetchFlow = async (id: string) => {
    try {
      setIsLoading(true)
      setError(null)

      const response = await fetch(`/self-service/registration/flows?id=${id}`, {
        method: 'GET',
        credentials: 'include',
        headers: {
          'Accept': 'application/json',
        },
      })

      if (!response.ok) {
        if (response.status === 404 || response.status === 410) {
          // Flow expired or not found, initiate new flow
          initiateRegistrationFlow()
          return
        }
        throw new Error(`Failed to fetch registration flow: ${response.status}`)
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

      const response = await fetch(`/self-service/registration?flow=${flow.id}`, {
        method: 'POST',
        credentials: 'include',
        headers: {
          'Content-Type': 'application/json',
          'Accept': 'application/json',
        },
        body: JSON.stringify({
          method: 'password',
          ...formData
        })
      })

      if (response.status === 303) {
        // Successful registration, follow redirect
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
        throw new Error(`Registration failed: ${response.status}`)
      }

      // Successful registration
      router.push(returnUrl)

    } catch (err) {
      setError(err instanceof Error ? err.message : 'Registration failed')
    } finally {
      setIsLoading(false)
    }
  }

  const renderFormField = (node: RegistrationFlowNode) => {
    if (node.type !== 'input' || node.group !== 'password') {
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
              <Alert status="error" mb={4} borderRadius="md">
                <Text fontSize="sm">{error}</Text>
              </Alert>
            )}

            {flow && (
              <form onSubmit={handleSubmit}>
                <VStack gap={4}>
                  {flow.ui.messages?.map((message, idx) => (
                    <Alert
                      key={idx}
                      status={message.type === 'error' ? 'error' : 'info'}
                      borderRadius="md"
                    >
                      <Text fontSize="sm">{message.text}</Text>
                    </Alert>
                  ))}

                  {flow.ui.nodes.map(renderFormField)}

                  <Button
                    type="submit"
                    w="full"
                    bg="var(--alt-primary)"
                    color="white"
                    size="lg"
                    isLoading={isLoading}
                    loadingText="登録中..."
                    _hover={{ bg: 'var(--alt-primary-hover)' }}
                    _active={{ bg: 'var(--alt-primary-active)' }}
                  >
                    新規登録
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
                onClick={() => router.push('/auth/login')}
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