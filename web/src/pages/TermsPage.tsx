import { useEffect, useState } from 'react'
import { Link } from 'react-router'
import { getTerms } from '../lib/api'
import { Footer } from '../components/Footer'

export default function TermsPage() {
  const [content, setContent] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    getTerms()
      .then((data) => setContent(data.content))
      .catch(() => setError('Failed to load terms and conditions.'))
  }, [])

  return (
    <div className="min-h-screen bg-gray-50 flex flex-col">
      <header className="bg-white border-b border-gray-200 px-4 py-5">
        <div className="max-w-2xl mx-auto">
          <h1 className="text-2xl font-bold text-gray-900">Terms &amp; Conditions</h1>
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
          <p className="text-gray-500">Terms and conditions have not been published yet.</p>
        ) : (
          <div className="bg-white rounded-2xl border border-gray-200 p-6">
            <pre className="whitespace-pre-wrap text-sm text-gray-800 font-sans leading-relaxed">
              {content}
            </pre>
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
