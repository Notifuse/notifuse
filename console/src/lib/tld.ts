// Function to extract TLD from URL
const extractTLD = (url: string): string => {
  try {
    if (!url || !url.trim()) return ''

    // Add protocol if missing to make URL parsing work
    const urlWithProtocol = url.startsWith('http') ? url : `https://${url}`
    const hostname = new URL(urlWithProtocol).hostname

    // Split by dots and get the last two parts (or just the last if it's a simple domain)
    const parts = hostname.split('.')
    if (parts.length >= 2) {
      return parts.slice(-2).join('.')
    }
    return hostname
  } catch (e) {
    console.error('Error extracting TLD:', e)
    return ''
  }
}

export default extractTLD
