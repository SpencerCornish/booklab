import DOMPurify from 'dompurify'

function looksLikeHtml(content: string): boolean {
  return /<[a-z][\s\S]*>/i.test(content)
}

function sanitizeHtml(html: string): string {
  return DOMPurify.sanitize(html, {
    ALLOWED_TAGS: [
      'p', 'br', 'strong', 'em', 'b', 'i', 'u',
      'h1', 'h2', 'h3', 'ul', 'ol', 'li', 'a', 'blockquote',
    ],
    ALLOWED_ATTR: ['href', 'target', 'rel'],
  })
}

export function RichTextContent({ content }: { content: string }) {
  if (!looksLikeHtml(content)) {
    return (
      <pre className="whitespace-pre-wrap text-sm text-gray-800 font-sans leading-relaxed">
        {content}
      </pre>
    )
  }

  return (
    <div
      className="rich-text-content text-sm text-gray-800 leading-relaxed"
      dangerouslySetInnerHTML={{ __html: sanitizeHtml(content) }}
    />
  )
}
