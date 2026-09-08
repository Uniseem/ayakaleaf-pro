import { redirect } from 'next/navigation'
import { currentUser } from '@/lib/auth'
import { forwardedHeaders } from '@/lib/server'

// The root is not a page. Somebody signed in wants their projects; somebody
// who is not wants a way in.
export default async function Home() {
  const user = await currentUser(await forwardedHeaders()).catch(() => null)
  redirect(user ? '/projects' : '/login')
}
