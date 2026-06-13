import { useEffect, useState } from 'react'
import { Link } from 'react-router'
import { getPrivacy } from '../lib/api'
import { Footer } from '../components/Footer'
import { RichTextContent } from '../components/RichTextContent'

export default function PrivacyPage() {
  const [content, setContent] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    getPrivacy()
      .then((data) => setContent(data.content))
      .catch(() => setError('Failed to load privacy policy.'))
  }, [])

  return (
    <div className="min-h-screen bg-gray-50 flex flex-col">
      <header className="bg-white border-b border-gray-200 px-4 py-5">
        <div className="max-w-2xl mx-auto">
          <h1 className="text-2xl font-bold text-gray-900">Privacy Policy</h1>
        </div>
      </header>

      <main className="flex-1 max-w-2xl mx-auto w-full px-4 py-8">
        {error ? (
          <p className="text-gray-500">{error}</p>
        ) : content === null ? (
          <div className="flex justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600" />
          </div>
        ) : content.trim() === '' ? (
          <p className="text-gray-500">Privacy policy has not been published yet.</p>
        ) : (
          <div className="bg-white rounded-2xl border border-gray-200 p-6">
            <RichTextContent content={content} />
          </div>
        )}

        <Link to="/" className="mt-6 inline-block text-sm text-blue-600 hover:underline">
          Back to booking
        </Link>
      </main>

      <Footer />
    </div>
  )
}
