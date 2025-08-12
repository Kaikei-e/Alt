"use client";

import { useState, useEffect } from 'react';
import {
  Box,
  Button,
  Field,
  Input,
  VStack,
  Text,
  IconButton,
  Flex,
  Spinner,
  HStack,
} from '@chakra-ui/react';
import { Eye, EyeOff, User, Mail, Lock, RefreshCw, AlertCircle, Shield, CheckCircle } from 'lucide-react';
import { useAuth } from '@/contexts/auth-context';
import { 
  validateEmail, 
  validatePassword, 
  validateName, 
  RateLimiter,
  sanitizeInput
} from '@/lib/security/security-utils';

type AuthMode = 'login' | 'register';

interface AuthFormProps {
  onSuccess?: () => void;
  initialMode?: AuthMode;
}

export function AuthForm({ onSuccess, initialMode = 'login' }: AuthFormProps) {
  const { login, register, isLoading, error, clearError, retryLastAction } = useAuth();
  const [mode, setMode] = useState<AuthMode>(initialMode);
  const [showPassword, setShowPassword] = useState(false);
  const [formData, setFormData] = useState({
    email: '',
    password: '',
    name: '',
  });
  
  // Validation states
  const [validationErrors, setValidationErrors] = useState<{
    email?: string;
    password?: string;
    name?: string;
  }>({});
  
  const [passwordStrength, setPasswordStrength] = useState<'weak' | 'medium' | 'strong'>('weak');
  const [isRateLimited, setIsRateLimited] = useState(false);
  const [rateLimitInfo, setRateLimitInfo] = useState<{ attemptsRemaining: number; resetTime?: number }>({
    attemptsRemaining: 5
  });
  
  // Rate limiter instance
  const [rateLimiter] = useState(() => new RateLimiter());

  const handleInputChange = (field: keyof typeof formData) => (
    e: React.ChangeEvent<HTMLInputElement>
  ) => {
    const value = sanitizeInput(e.target.value);
    
    setFormData(prev => ({
      ...prev,
      [field]: value,
    }));
    
    // Real-time validation
    validateField(field, value);
    
    // Clear global error when user starts typing
    if (error) {
      clearError();
    }
  };
  
  const validateField = (field: keyof typeof formData, value: string) => {
    let fieldError: string | undefined;
    
    switch (field) {
      case 'email':
        const emailValidation = validateEmail(value);
        fieldError = emailValidation.isValid ? undefined : emailValidation.error;
        break;
        
      case 'password':
        const passwordValidation = validatePassword(value);
        fieldError = passwordValidation.isValid ? undefined : passwordValidation.error;
        setPasswordStrength(passwordValidation.strength);
        break;
        
      case 'name':
        const nameValidation = validateName(value);
        fieldError = nameValidation.isValid ? undefined : nameValidation.error;
        break;
    }
    
    setValidationErrors(prev => ({
      ...prev,
      [field]: fieldError,
    }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    // Check rate limiting
    const rateLimitResult = rateLimiter.recordAttempt(formData.email);
    setRateLimitInfo(rateLimitResult);
    
    if (!rateLimitResult.allowed) {
      setIsRateLimited(true);
      return;
    }
    
    // Validate all fields before submission
    const allFieldsValid = validateAllFields();
    if (!allFieldsValid) {
      return;
    }
    
    try {
      if (mode === 'login') {
        await login(formData.email, formData.password);
      } else {
        await register(formData.email, formData.password, formData.name);
      }
      
      // Reset rate limiter on success
      rateLimiter.reset(formData.email);
      setIsRateLimited(false);
      
      // Clear form on success
      setFormData({ email: '', password: '', name: '' });
      setValidationErrors({});
      
      // Call success callback
      onSuccess?.();
    } catch (err) {
      // Error is handled by the auth context
      console.error('Authentication error:', err);
    }
  };
  
  const validateAllFields = (): boolean => {
    const emailValidation = validateEmail(formData.email);
    const passwordValidation = validatePassword(formData.password);
    const nameValidation = mode === 'register' ? validateName(formData.name) : { isValid: true };
    
    const errors: typeof validationErrors = {};
    
    if (!emailValidation.isValid) {
      errors.email = emailValidation.error;
    }
    
    if (!passwordValidation.isValid) {
      errors.password = passwordValidation.error;
    }
    
    if (mode === 'register' && !nameValidation.isValid) {
      errors.name = nameValidation.error;
    }
    
    setValidationErrors(errors);
    setPasswordStrength(passwordValidation.strength);
    
    return Object.keys(errors).length === 0;
  };

  const handleModeToggle = () => {
    const newMode = mode === 'login' ? 'register' : 'login';
    setMode(newMode);
    clearError();
    setFormData({ email: '', password: '', name: '' });
  };

  const isFormValid = () => {
    const hasValidationErrors = Object.values(validationErrors).some(error => error);
    
    if (mode === 'login') {
      return formData.email.length > 0 && 
             formData.password.length > 0 && 
             !hasValidationErrors &&
             !isRateLimited;
    } else {
      return formData.email.length > 0 && 
             formData.password.length > 0 && 
             formData.name.length > 0 && 
             !hasValidationErrors &&
             !isRateLimited;
    }
  };
  
  const getPasswordStrengthColor = () => {
    switch (passwordStrength) {
      case 'weak': return 'semantic.error';
      case 'medium': return 'semantic.warning';
      case 'strong': return 'semantic.success';
    }
  };
  
  const getPasswordStrengthText = () => {
    switch (passwordStrength) {
      case 'weak': return '弱い';
      case 'medium': return '普通';
      case 'strong': return '強い';
    }
  };

  const handleRetry = async () => {
    try {
      await retryLastAction();
      // Clear form on success
      setFormData({ email: '', password: '', name: '' });
      onSuccess?.();
    } catch (err) {
      console.error('Retry failed:', err);
    }
  };

  const getErrorIcon = () => {
    if (!error) return null;
    
    switch (error.type) {
      case 'NETWORK_ERROR':
      case 'TIMEOUT_ERROR':
        return <RefreshCw size={16} />;
      default:
        return <AlertCircle size={16} />;
    }
  };

  const getErrorColor = () => {
    if (!error) return 'semantic.error';
    
    switch (error.type) {
      case 'NETWORK_ERROR':
      case 'TIMEOUT_ERROR':
        return 'semantic.warning';
      default:
        return 'semantic.error';
    }
  };

  return (
    <Box
      bg="var(--alt-glass)"
      border="1px solid"
      borderColor="var(--alt-glass-border)"
      backdropFilter="blur(12px) saturate(1.2)"
      p={6}
      borderRadius="lg"
      w="full"
      maxW="400px"
      mx="auto"
      position="relative"
      overflow="hidden"
    >
      {/* Subtle gradient overlay */}
      <Box
        position="absolute"
        top={0}
        left={0}
        right={0}
        bottom={0}
        bgGradient="linear(135deg, transparent 0%, var(--alt-glass) 50%, transparent 100%)"
        opacity={0.5}
        pointerEvents="none"
      />

      <Box position="relative" zIndex={1}>
        {/* Mode Toggle */}
        <HStack
          bg="var(--alt-glass)"
          p={1}
          borderRadius="md"
          mb={6}
          gap={0}
        >
          <Button
            flex={1}
            variant={mode === 'login' ? 'solid' : 'ghost'}
            bg={mode === 'login' ? "var(--alt-primary)" : "transparent"}
            color={mode === 'login' ? "white" : "var(--text-primary)"}
            fontFamily="heading"
            fontWeight="semibold"
            onClick={() => mode !== 'login' && handleModeToggle()}
            _hover={{
              bg: mode === 'login' ? "var(--alt-primary)" : "var(--alt-glass)"
            }}
          >
            ログイン
          </Button>
          <Button
            flex={1}
            variant={mode === 'register' ? 'solid' : 'ghost'}
            bg={mode === 'register' ? "var(--alt-primary)" : "transparent"}
            color={mode === 'register' ? "white" : "var(--text-primary)"}
            fontFamily="heading"
            fontWeight="semibold"
            onClick={() => mode !== 'register' && handleModeToggle()}
            _hover={{
              bg: mode === 'register' ? "var(--alt-primary)" : "var(--alt-glass)"
            }}
          >
            新規登録
          </Button>
        </HStack>

        <form onSubmit={handleSubmit}>
          <VStack gap={4}>
            {/* Rate limiting warning */}
            {isRateLimited && (
              <Box
                bg="semantic.warning"
                border="1px solid"
                borderColor="semantic.warning"
                borderRadius="md"
                p={3}
                w="full"
              >
                <VStack gap={2} align="stretch">
                  <Flex align="center" gap={2}>
                    <Shield size={16} color="white" />
                    <Text color="white" fontSize="sm" fontWeight="semibold" fontFamily="heading">
                      アクセス制限
                    </Text>
                  </Flex>
                  <Text color="white" fontSize="xs" fontFamily="body">
                    ログイン試行回数が上限に達しました。
                    {rateLimitInfo.resetTime && (
                      ` ${Math.ceil((rateLimitInfo.resetTime - Date.now()) / 60000)}分後に再試行できます。`
                    )}
                  </Text>
                  {rateLimitInfo.attemptsRemaining > 0 && (
                    <Text color="white" fontSize="xs" fontFamily="body">
                      残り試行回数: {rateLimitInfo.attemptsRemaining}
                    </Text>
                  )}
                </VStack>
              </Box>
            )}
            
            {error && (
              <Box
                bg="var(--alt-glass)"
                border="1px solid"
                borderColor={getErrorColor()}
                borderRadius="md"
                p={3}
                w="full"
              >
                <VStack gap={2} align="stretch">
                  <Flex align="center" gap={2}>
                    <Box color={getErrorColor()}>
                      {getErrorIcon()}
                    </Box>
                    <Text color={getErrorColor()} fontSize="sm" fontFamily="body" flex={1}>
                      {error.message}
                    </Text>
                  </Flex>
                  
                  {error.isRetryable && (
                    <Flex justify="space-between" align="center">
                      <Text fontSize="xs" color="var(--text-muted)" fontFamily="body">
                        {error.retryCount !== undefined && error.retryCount > 0 
                          ? `再試行回数: ${error.retryCount}/3`
                          : '再試行可能'
                        }
                      </Text>
                      <Button
                        size="sm"
                        variant="outline"
                        bg="var(--alt-glass)"
                        borderColor={getErrorColor()}
                        color={getErrorColor()}
                        onClick={handleRetry}
                        disabled={isLoading}
                        _hover={{
                          bg: getErrorColor(),
                          color: "white",
                        }}
                      >
                        <Flex align="center" gap={1}>
                          <RefreshCw size={12} />
                          <Text fontSize="xs">再試行</Text>
                        </Flex>
                      </Button>
                    </Flex>
                  )}
                </VStack>
              </Box>
            )}

            {mode === 'register' && (
              <Field.Root>
                <Field.Label>
                  <Flex align="center" gap={2} color="var(--text-primary)" fontFamily="body" fontSize="sm">
                    <User size={16} />
                    お名前
                    {formData.name && !validationErrors.name && (
                      <CheckCircle size={12} color="green" />
                    )}
                  </Flex>
                </Field.Label>
                <Input
                  type="text"
                  value={formData.name}
                  onChange={handleInputChange('name')}
                  placeholder="山田太郎"
                  bg="var(--alt-glass)"
                  border="1px solid"
                  borderColor={validationErrors.name ? "semantic.error" : "var(--alt-glass-border)"}
                  color="var(--text-primary)"
                  _placeholder={{ color: "var(--text-muted)" }}
                  _hover={{ borderColor: validationErrors.name ? "semantic.error" : "var(--alt-primary)" }}
                  _focus={{
                    borderColor: validationErrors.name ? "semantic.error" : "var(--alt-primary)",
                    boxShadow: validationErrors.name ? "0 0 0 1px semantic.error" : "0 0 0 1px var(--alt-primary)",
                  }}
                />
                {validationErrors.name && (
                  <Text color="semantic.error" fontSize="xs" fontFamily="body" mt={1}>
                    {validationErrors.name}
                  </Text>
                )}
              </Field.Root>
            )}

            <Field.Root>
              <Field.Label>
                <Flex align="center" gap={2} color="var(--text-primary)" fontFamily="body" fontSize="sm">
                  <Mail size={16} />
                  メールアドレス
                  {formData.email && !validationErrors.email && (
                    <CheckCircle size={12} color="green" />
                  )}
                </Flex>
              </Field.Label>
              <Input
                type="email"
                value={formData.email}
                onChange={handleInputChange('email')}
                placeholder="you@example.com"
                bg="var(--alt-glass)"
                border="1px solid"
                borderColor={validationErrors.email ? "semantic.error" : "var(--alt-glass-border)"}
                color="var(--text-primary)"
                _placeholder={{ color: "var(--text-muted)" }}
                _hover={{ borderColor: validationErrors.email ? "semantic.error" : "var(--alt-primary)" }}
                _focus={{
                  borderColor: validationErrors.email ? "semantic.error" : "var(--alt-primary)",
                  boxShadow: validationErrors.email ? "0 0 0 1px semantic.error" : "0 0 0 1px var(--alt-primary)",
                }}
              />
              {validationErrors.email && (
                <Text color="semantic.error" fontSize="xs" fontFamily="body" mt={1}>
                  {validationErrors.email}
                </Text>
              )}
            </Field.Root>

            <Field.Root>
              <Field.Label>
                <Flex align="center" gap={2} color="var(--text-primary)" fontFamily="body" fontSize="sm">
                  <Lock size={16} />
                  パスワード
                  {formData.password && !validationErrors.password && (
                    <CheckCircle size={12} color="green" />
                  )}
                </Flex>
              </Field.Label>
              <VStack gap={2} align="stretch">
                <Flex position="relative">
                  <Input
                    type={showPassword ? 'text' : 'password'}
                    value={formData.password}
                    onChange={handleInputChange('password')}
                    placeholder={mode === 'register' ? "8文字以上" : "••••••••"}
                    bg="var(--alt-glass)"
                    border="1px solid"
                    borderColor={validationErrors.password ? "semantic.error" : "var(--alt-glass-border)"}
                    color="var(--text-primary)"
                    _placeholder={{ color: "var(--text-muted)" }}
                    _hover={{ borderColor: validationErrors.password ? "semantic.error" : "var(--alt-primary)" }}
                    _focus={{
                      borderColor: validationErrors.password ? "semantic.error" : "var(--alt-primary)",
                      boxShadow: validationErrors.password ? "0 0 0 1px semantic.error" : "0 0 0 1px var(--alt-primary)",
                    }}
                    pr="3rem"
                  />
                  <IconButton
                    position="absolute"
                    right="0.5rem"
                    top="50%"
                    transform="translateY(-50%)"
                    aria-label={showPassword ? 'パスワードを隠す' : 'パスワードを表示'}
                    variant="ghost"
                    size="sm"
                    color="var(--text-muted)"
                    onClick={() => setShowPassword(!showPassword)}
                    _hover={{ color: "var(--alt-primary)" }}
                  >
                    {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                  </IconButton>
                </Flex>
                
                {/* Password strength indicator for registration */}
                {mode === 'register' && formData.password && (
                  <Flex align="center" justify="space-between">
                    <Text fontSize="xs" color="var(--text-muted)" fontFamily="body">
                      パスワード強度:
                    </Text>
                    <Text 
                      fontSize="xs" 
                      color={getPasswordStrengthColor()} 
                      fontWeight="semibold"
                      fontFamily="body"
                    >
                      {getPasswordStrengthText()}
                    </Text>
                  </Flex>
                )}
                
                {validationErrors.password && (
                  <Text color="semantic.error" fontSize="xs" fontFamily="body">
                    {validationErrors.password}
                  </Text>
                )}
              </VStack>
            </Field.Root>

            <Button
              type="submit"
              w="full"
              bg="var(--alt-primary)"
              color="white"
              fontFamily="heading"
              fontWeight="semibold"
              loading={isLoading}
              disabled={!isFormValid()}
              _hover={{
                bg: "var(--alt-primary)",
                transform: "translateY(-1px)",
                boxShadow: "0 4px 12px var(--alt-glass-shadow)",
              }}
              _active={{
                transform: "translateY(0)",
              }}
              _disabled={{
                opacity: 0.5,
                cursor: "not-allowed",
                _hover: {
                  transform: "none",
                  boxShadow: "none",
                },
              }}
            >
              {isLoading ? (
                <Flex align="center" gap={2}>
                  <Spinner size="sm" />
                  {mode === 'login' ? '認証中...' : '登録中...'}
                </Flex>
              ) : (
                mode === 'login' ? 'ログイン' : 'アカウント作成'
              )}
            </Button>
          </VStack>
        </form>

        <Text
          mt={4}
          fontSize="xs"
          color="var(--text-muted)"
          textAlign="center"
          fontFamily="body"
        >
          続行することで、利用規約とプライバシーポリシーに同意したものとみなされます
        </Text>
      </Box>
    </Box>
  );
}