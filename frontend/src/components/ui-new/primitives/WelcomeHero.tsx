import { useTranslation } from 'react-i18next';

/**
 * SoloDawn "dawn mark" — a sunrise cresting a horizon line with radiating
 * rays, drawn in the brand orange. Pairs with the wordmark below.
 */
function DawnMark({ className }: Readonly<{ className?: string }>) {
  return (
    <svg
      viewBox="0 0 64 64"
      role="img"
      aria-label="SoloDawn"
      className={className}
      fill="none"
    >
      {/* Soft halo glow behind the sun */}
      <circle
        cx="32"
        cy="41"
        r="20"
        className="hero-mark-halo"
        fill="hsl(var(--brand) / 0.16)"
      />
      {/* Radiating rays */}
      <g
        className="hero-mark-rays"
        stroke="hsl(var(--brand))"
        strokeWidth="2.4"
        strokeLinecap="round"
      >
        <line x1="32" y1="14" x2="32" y2="4" />
        <line x1="14.7" y1="23.7" x2="7.6" y2="16.6" />
        <line x1="49.3" y1="23.7" x2="56.4" y2="16.6" />
      </g>
      {/* The cresting sun */}
      <path
        d="M14 41a18 18 0 0 1 36 0"
        stroke="hsl(var(--brand))"
        strokeWidth="3.2"
        strokeLinecap="round"
        fill="hsl(var(--brand) / 0.22)"
      />
      {/* Horizon line */}
      <line
        x1="6"
        y1="41"
        x2="58"
        y2="41"
        stroke="hsl(var(--text-high))"
        strokeWidth="2.6"
        strokeLinecap="round"
      />
    </svg>
  );
}

export function WelcomeHero() {
  const { t } = useTranslation('tasks');

  return (
    <div className="flex w-chat max-w-full flex-col items-center px-base text-center">
      {/* Mark + wordmark */}
      <div className="hero-mark mb-double flex flex-col items-center gap-base">
        <DawnMark className="size-12" />
        <div className="font-space-grotesk text-xl font-semibold tracking-tight text-high">
          Solo<span className="text-brand">Dawn</span>
        </div>
      </div>

      {/* Eyebrow tag */}
      <span
        className="hero-reveal mb-base inline-flex items-center gap-half rounded-sm border border-brand/30 bg-brand/[0.07] px-base py-half font-ibm-plex-mono text-xs uppercase tracking-[0.22em] text-brand"
        style={{ '--hero-i': 1 } as React.CSSProperties}
      >
        <span className="size-dot rounded-full bg-brand" />
        {t('conversation.createLanding.eyebrow')}
      </span>

      {/* Headline */}
      <h1
        className="hero-reveal max-w-[34rem] font-space-grotesk text-[2rem] font-bold leading-[1.08] tracking-tight text-high"
        style={{ '--hero-i': 2 } as React.CSSProperties}
      >
        {t('conversation.createLanding.headline')}{' '}
        <span className="text-brand">
          {t('conversation.createLanding.headlineAccent')}
        </span>
      </h1>

      {/* Subline */}
      <p
        className="hero-reveal mt-base max-w-[30rem] font-ibm-plex-sans text-lg leading-relaxed text-normal"
        style={{ '--hero-i': 3 } as React.CSSProperties}
      >
        {t('conversation.createLanding.subline')}
      </p>
    </div>
  );
}
