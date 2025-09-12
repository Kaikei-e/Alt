import { redirect } from "next/navigation";

export default function RootPage() {
  // Middleware で認証済みが保証されているため、
  // 直接ホームページにリダイレクト
  redirect("/home");
}
