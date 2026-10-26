export function BrandLogo() {
  return (
    <span className="brand-lockup">
      <span className="brand-mark" aria-hidden="true">
        <img
          src={`${import.meta.env.BASE_URL}brand/tastelingo-mark-3d.webp`}
          alt=""
          width="128"
          height="128"
          decoding="async"
          fetchPriority="high"
        />
      </span>
      <span className="brand-copy">
        <span className="brand-wordmark">Taste<span>Lingo</span><span className="brand-period" aria-hidden="true">.</span></span>
        <span className="brand-tagline">Your taste. Your way to English.</span>
      </span>
    </span>
  )
}
