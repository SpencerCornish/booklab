export function Footer() {
  return (
    <footer className="border-t border-gray-200 bg-white px-4 py-4 mt-auto">
      <div className="max-w-2xl mx-auto text-center text-sm text-gray-500">
        <a
          href="/terms"
          target="_blank"
          rel="noopener noreferrer"
          className="text-gray-600 hover:text-gray-900 hover:underline"
        >
          Terms &amp; Conditions
        </a>
        <span className="mx-2">·</span>
        <a
          href="/privacy"
          target="_blank"
          rel="noopener noreferrer"
          className="text-gray-600 hover:text-gray-900 hover:underline"
        >
          Privacy Policy
        </a>
      </div>
    </footer>
  )
}
