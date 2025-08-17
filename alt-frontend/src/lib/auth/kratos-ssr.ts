import { GetServerSidePropsContext, GetServerSidePropsResult } from 'next'

export interface AuthUser {
  id: string
  email: string
  name?: string
}

export interface AuthSession {
  identity: AuthUser
  authenticated_at: string
  expires_at: string
}

/**
 * SSR authentication guard using Kratos whoami endpoint
 * TODO.mdに従った実装: サーバーサイドでwhoamiを見て、未ログインならリダイレクト
 */
export async function withAuth<P extends Record<string, any> = Record<string, any>>(
  getServerSideProps?: (
    context: GetServerSidePropsContext,
    user: AuthUser
  ) => Promise<GetServerSidePropsResult<P>> | GetServerSidePropsResult<P>
): Promise<(context: GetServerSidePropsContext) => Promise<GetServerSidePropsResult<P | {}>>> {
  return async (context: GetServerSidePropsContext): Promise<GetServerSidePropsResult<P | {}>> => {
    const { req } = context
    
    try {
      // TODO.mdの要求：サーバサイドでwhoamiを呼び出し、必ずCookieを前方転送
      const response = await fetch('https://id.curionoah.com/sessions/whoami', {
        headers: { 
          cookie: req.headers.cookie ?? '', // 必ずCookieを前方転送
          'Accept': 'application/json'
        },
        redirect: 'manual', // 手動でリダイレクトを処理
      })

      // TODO.mdの要求：未ログインならリダイレクト
      if (response.status !== 200) {
        const currentPath = context.resolvedUrl || context.req.url || '/'
        const loginUrl = `/auth/login?returnUrl=${encodeURIComponent(currentPath)}`
        
        return {
          redirect: {
            destination: loginUrl,
            permanent: false,
          },
        }
      }

      const session: AuthSession = await response.json()
      
      if (!session.identity) {
        const currentPath = context.resolvedUrl || context.req.url || '/'
        const loginUrl = `/auth/login?returnUrl=${encodeURIComponent(currentPath)}`
        
        return {
          redirect: {
            destination: loginUrl,
            permanent: false,
          },
        }
      }

      // 認証済みの場合、元のgetServerSidePropsを実行
      if (getServerSideProps) {
        return await getServerSideProps(context, session.identity)
      }

      // デフォルトではユーザー情報をpropsで渡す
      return {
        props: {
          user: session.identity,
        } as P,
      }

    } catch (error) {
      console.error('Authentication check failed:', error)
      
      // エラーの場合もログインページにリダイレクト
      const currentPath = context.resolvedUrl || context.req.url || '/'
      const loginUrl = `/auth/login?returnUrl=${encodeURIComponent(currentPath)}`
      
      return {
        redirect: {
          destination: loginUrl,
          permanent: false,
        },
      }
    }
  }
}

/**
 * Client-side authentication check using Kratos whoami
 */
export async function checkAuthStatus(): Promise<AuthUser | null> {
  try {
    const response = await fetch('/sessions/whoami', {
      credentials: 'include',
      headers: {
        'Accept': 'application/json',
      },
    })

    if (response.status !== 200) {
      return null
    }

    const session: AuthSession = await response.json()
    return session.identity || null

  } catch (error) {
    console.error('Client auth check failed:', error)
    return null
  }
}

/**
 * Logout function that calls Kratos logout endpoint
 */
export async function logout(): Promise<void> {
  try {
    const response = await fetch('/self-service/logout/browser', {
      method: 'GET',
      credentials: 'include',
    })

    if (response.status === 303) {
      // Follow redirect to complete logout
      const location = response.headers.get('Location')
      if (location) {
        window.location.href = location
        return
      }
    }

    // Fallback: redirect to login page
    window.location.href = '/auth/login'

  } catch (error) {
    console.error('Logout failed:', error)
    // Force redirect to login even on error
    window.location.href = '/auth/login'
  }
}