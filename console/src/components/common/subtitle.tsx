const Subtitle = ({
  className,
  children,
  borderBottom = false,
  primary
}: {
  className?: string
  children: React.ReactNode
  borderBottom?: boolean
  primary?: boolean
}) => {
  // Detect if primary is set; true if defined (not undefined), false if not
  const isPrimary = typeof primary !== 'undefined' ? !!primary : false

  const base = 'text-xs font-medium mb-2'
  const text = isPrimary ? 'text-primary' : ''
  let border = ''
  if (borderBottom) {
    border = isPrimary ? 'border-b border-primary-300 pb-2' : 'border-b border-gray-300 pb-2'
  }

  return <div className={`${base} ${className ?? ''} ${border} ${text}`}>{children}</div>
}

export default Subtitle
