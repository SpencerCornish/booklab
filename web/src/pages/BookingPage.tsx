import { useEffect, useState } from 'react'
import { format } from 'date-fns'
import { loadStripe } from '@stripe/stripe-js'
import { Elements, CardElement, useStripe, useElements } from '@stripe/react-stripe-js'
import { DatePicker } from '../components/DatePicker'
import { TimeSlotPicker } from '../components/TimeSlotPicker'
import { Footer } from '../components/Footer'
import {
  getPublicSettings,
  getAvailability,
  createBooking,
  cancelBooking,
  confirmBookingCard,
  ApiError,
} from '../lib/api'
import type { PublicSettings, AvailabilityResponse, BookingPublic } from '../lib/types'

// Stripe publishable key from env (set in .env or via Vite)
const stripePromise = loadStripe(import.meta.env.VITE_STRIPE_PUBLISHABLE_KEY ?? '')

export default function BookingPage() {
  const [settings, setSettings] = useState<PublicSettings | null>(null)

  useEffect(() => {
    getPublicSettings().then(setSettings).catch(console.error)
  }, [])

  if (!settings) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600" />
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-gray-50 flex flex-col">
      <header className="bg-white border-b border-gray-200 px-4 py-5">
        <div className="max-w-2xl mx-auto">
          <h1 className="text-2xl font-bold text-gray-900">Book {settings.resource_name}</h1>
          <p className="text-sm text-gray-500 mt-1">
            ${(settings.hourly_rate_cents / 100).toFixed(0)}/hr · Payment collected after your session
          </p>
        </div>
      </header>

      <div className="flex-1">
        <Elements stripe={stripePromise}>
          <BookingForm settings={settings} />
        </Elements>
      </div>

      <Footer />
    </div>
  )
}

function BookingForm({ settings }: { settings: PublicSettings }) {
  const stripe = useStripe()
  const elements = useElements()

  const today = format(new Date(), 'yyyy-MM-dd')
  const [date, setDate] = useState(today)
  const [availability, setAvailability] = useState<AvailabilityResponse | null>(null)
  const [loadingSlots, setLoadingSlots] = useState(false)
  const [selectedStart, setSelectedStart] = useState<string | null>(null)
  const [selectedEnd, setSelectedEnd] = useState<string | null>(null)

  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [phone, setPhone] = useState('')
  const [staffNotes, setStaffNotes] = useState('')
  const [referralSource, setReferralSource] = useState('')
  const [referralSourceOther, setReferralSourceOther] = useState('')
  const [acceptedTerms, setAcceptedTerms] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<BookingPublic | null>(null)

  useEffect(() => {
    setLoadingSlots(true)
    setSelectedStart(null)
    setSelectedEnd(null)
    getAvailability(date)
      .then(setAvailability)
      .catch(console.error)
      .finally(() => setLoadingSlots(false))
  }, [date])

  const handleSlotSelect = (start: string, end: string) => {
    setSelectedStart(start)
    setSelectedEnd(end)
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!stripe || !elements || !selectedStart || !selectedEnd) return

    setSubmitting(true)
    setError(null)

    let createdBooking: Awaited<ReturnType<typeof createBooking>>['booking'] | null = null
    try {
      const metadata: Record<string, string> = {}
      if (phone) metadata['Phone'] = phone
      if (staffNotes) metadata['Notes for staff'] = staffNotes
      if (referralSource) {
        metadata['ReferralSource'] = referralSource
        if (referralSource.toLowerCase() === 'other' && referralSourceOther.trim()) {
          metadata['ReferralSourceOther'] = referralSourceOther.trim()
        }
      }

      const { booking, setup_intent_client_secret } = await createBooking({
        name,
        email,
        metadata,
        start_time: selectedStart,
        end_time: selectedEnd,
      })
      createdBooking = booking

      const cardElement = elements.getElement(CardElement)
      if (!cardElement) throw new Error('Card element not found')

      const { error: stripeError } = await stripe.confirmCardSetup(setup_intent_client_secret, {
        payment_method: { card: cardElement, billing_details: { name, email } },
      })

      if (stripeError) {
        // Card setup failed - release the time slot by cancelling the booking
        cancelBooking(booking.cancel_token).catch(() => {})
        setError(stripeError.message ?? 'Card setup failed')
        return
      }

      // Persist the payment method to the booking so admin can charge later
      await confirmBookingCard(booking.cancel_token)

      setSuccess(booking)
    } catch (err) {
      // If booking was created but something else failed, clean it up
      if (createdBooking && !(err instanceof ApiError)) {
        cancelBooking(createdBooking.cancel_token).catch(() => {})
      }
      setError(err instanceof ApiError ? err.message : 'Something went wrong')
    } finally {
      setSubmitting(false)
    }
  }

  if (success) {
    return (
      <div className="max-w-2xl mx-auto px-4 py-12 text-center">
        <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-8">
          <div className="w-16 h-16 rounded-full bg-green-100 flex items-center justify-center mx-auto mb-4">
            <svg className="w-8 h-8 text-green-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
            </svg>
          </div>
          <h2 className="text-2xl font-bold text-gray-900 mb-2">You're booked!</h2>
          <p className="text-gray-500 mb-1">
            {format(new Date(success.start_time), 'EEEE, MMMM d, yyyy')}
          </p>
          <p className="text-gray-700 font-medium">
            {format(new Date(success.start_time), 'h:mm a')} – {format(new Date(success.end_time), 'h:mm a')}
          </p>
          <p className="text-sm text-gray-400 mt-4">
            A confirmation email has been sent to <strong>{success.email}</strong>.
            Payment will be collected after your session.
          </p>
        </div>
      </div>
    )
  }

  return (
    <form onSubmit={handleSubmit} className="max-w-2xl mx-auto px-4 py-8 space-y-8">
      {/* Date selection */}
      <section>
        <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wider mb-3">
          1. Choose a date
        </h2>
        <DatePicker selected={date} onChange={setDate} />
      </section>

      {/* Time slot selection */}
      <section>
        <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wider mb-3">
          2. Choose a time
        </h2>
        {loadingSlots ? (
          <div className="flex items-center gap-2 text-gray-400 py-4">
            <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-gray-400" />
            <span className="text-sm">Loading availability…</span>
          </div>
        ) : availability?.is_closed ? (
          <div className="rounded-xl bg-amber-50 border border-amber-200 px-4 py-3 text-amber-800 text-sm">
            This date is closed{availability.closure_reason ? `: ${availability.closure_reason}` : ''}.
          </div>
        ) : (
          <TimeSlotPicker
            slots={availability?.slots ?? []}
            selectedStart={selectedStart}
            selectedEnd={selectedEnd}
            minHours={settings.min_hours}
            maxHours={settings.max_hours}
            onSelect={handleSlotSelect}
            onClear={() => { setSelectedStart(null); setSelectedEnd(null) }}
          />
        )}
      </section>

      {/* Estimated total */}
      {selectedStart && selectedEnd && (() => {
        const hours = Math.max(1, Math.round(
          (new Date(selectedEnd).getTime() - new Date(selectedStart).getTime()) / 3600000
        ))
        const total = (hours * settings.hourly_rate_cents / 100).toFixed(2)
        return (
          <div className="rounded-xl bg-blue-50 border border-blue-100 px-4 py-3 flex items-center justify-between">
            <span className="text-sm text-blue-800 font-medium">
              Estimated total · {hours} hr{hours !== 1 ? 's' : ''}
            </span>
            <span className="text-lg font-bold text-blue-900">${total}</span>
          </div>
        )
      })()}

      {/* Booking details */}
      {selectedStart && selectedEnd && (
        <section>
          <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wider mb-3">
            3. Your details
          </h2>
          <div className="bg-white rounded-2xl border border-gray-200 p-6 space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
              <input
                type="text"
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                placeholder="Your full name"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Email</label>
              <input
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                placeholder="you@example.com"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Phone</label>
              <input
                type="tel"
                required
                value={phone}
                onChange={(e) => setPhone(e.target.value)}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                placeholder="(406) 555-0100"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Notes for staff <span className="text-gray-400 font-normal">(optional)</span></label>
              <textarea
                value={staffNotes}
                onChange={(e) => setStaffNotes(e.target.value)}
                rows={3}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none"
                placeholder="Anything we should know about your session…"
              />
            </div>

            {settings.referral_sources.length > 0 && (
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  How did you hear about us? <span className="text-gray-400 font-normal">(optional)</span>
                </label>
                <select
                  value={referralSource}
                  onChange={(e) => {
                    setReferralSource(e.target.value)
                    if (e.target.value.toLowerCase() !== 'other') {
                      setReferralSourceOther('')
                    }
                  }}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 bg-white"
                >
                  <option value="">Select one…</option>
                  {settings.referral_sources.map((source) => (
                    <option key={source} value={source}>
                      {source}
                    </option>
                  ))}
                </select>
                {referralSource.toLowerCase() === 'other' && (
                  <input
                    type="text"
                    value={referralSourceOther}
                    onChange={(e) => setReferralSourceOther(e.target.value)}
                    className="w-full mt-2 rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                    placeholder="Please tell us more…"
                  />
                )}
              </div>
            )}

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Card</label>
              <div className="rounded-lg border border-gray-300 px-3 py-3">
                <CardElement
                  options={{
                    style: {
                      base: { fontSize: '14px', color: '#374151', '::placeholder': { color: '#9ca3af' } },
                    },
                  }}
                />
              </div>
              <p className="text-xs text-gray-400 mt-1">
                Your card won't be charged until after your session.
              </p>
            </div>

            {error && (
              <div className="rounded-lg bg-red-50 border border-red-200 text-red-700 text-sm px-3 py-2">
                {error}
              </div>
            )}

            <label className="flex items-start gap-2 text-sm text-gray-700">
              <input
                type="checkbox"
                required
                checked={acceptedTerms}
                onChange={(e) => setAcceptedTerms(e.target.checked)}
                className="mt-0.5 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              <span>
                I accept the{' '}
                <a
                  href="/terms"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-blue-600 hover:underline"
                >
                  Terms &amp; Conditions
                </a>
                {' & '}
                <a
                  href="/privacy"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-blue-600 hover:underline"
                >
                  Privacy Policy
                </a>
              </span>
            </label>

            <button
              type="submit"
              disabled={submitting || !stripe || !acceptedTerms}
              className="w-full bg-blue-600 text-white rounded-lg py-2.5 font-medium text-sm hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {submitting ? 'Confirming…' : 'Confirm Booking'}
            </button>
          </div>
        </section>
      )}
    </form>
  )
}
