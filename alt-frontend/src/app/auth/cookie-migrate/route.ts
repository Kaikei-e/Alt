import { NextResponse } from 'next/server';

export async function GET() {
  const headers = new Headers();

  // 古い狭域Cookieの完全削除（存在してもしてなくてもOK）
  headers.append('Set-Cookie', 'ory_kratos_session=; Max-Age=0; Path=/; Domain=id.curionoah.com; HttpOnly; Secure; SameSite=Lax');
  // 念のため apex 側も一度消しておく
  headers.append('Set-Cookie', 'ory_kratos_session=; Max-Age=0; Path=/; Domain=curionoah.com; HttpOnly; Secure; SameSite=Lax');

  // 204で返す（UIには何も出さない）
  return new NextResponse(null, { status: 204, headers });
}