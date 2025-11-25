/**
 * YouTube URL utilities for parsing and transforming YouTube URLs
 * Shared between YoutubeExtension (for HTML serialization) and YoutubeNodeView (for editor display)
 *
 * Based on official @tiptap/extension-youtube implementation with enhancements
 */

export interface YoutubeEmbedOptions {
  cc?: boolean
  loop?: boolean
  controls?: boolean
  modestbranding?: boolean
  start?: number
}

/**
 * Comprehensive YouTube URL regex matching all formats
 * Matches: youtube.com, youtu.be, youtube-nocookie.com, music.youtube.com
 * Supports: /watch?v=, /embed/, /v/, /shorts/, youtu.be/
 */
export const YOUTUBE_REGEX =
  /^((?:https?:)?\/\/)?((?:www|m|music)\.)?((?:youtube\.com|youtu\.be|youtube-nocookie\.com))(\/(?:[\w-]+\?v=|embed\/|v\/|shorts\/)?)([\w-]+)(\S+)?$/

/**
 * Validate if a string is a valid YouTube URL
 *
 * @param url - String to validate
 * @returns true if valid YouTube URL, false otherwise
 */
export function isValidYoutubeUrl(url: string): boolean {
  if (!url) return false
  return YOUTUBE_REGEX.test(url)
}

/**
 * Extract YouTube video ID from various URL formats
 *
 * Supports:
 * - Standard watch URLs: https://www.youtube.com/watch?v=VIDEO_ID
 * - Short URLs: https://youtu.be/VIDEO_ID
 * - Embed URLs: https://www.youtube.com/embed/VIDEO_ID
 * - Shorts: https://www.youtube.com/shorts/VIDEO_ID
 * - Direct URLs: https://www.youtube.com/v/VIDEO_ID
 * - No-cookie URLs: https://www.youtube-nocookie.com/embed/VIDEO_ID
 * - Music URLs: https://music.youtube.com/watch?v=VIDEO_ID
 * - Video ID only: VIDEO_ID (11 characters)
 *
 * @param url - YouTube URL or video ID
 * @returns Video ID or null if invalid
 */
export function getYoutubeVideoId(url: string): string | null {
  if (!url) return null

  const trimmed = url.trim()

  // Just video ID (11 characters, alphanumeric plus - and _)
  if (/^[a-zA-Z0-9_-]{11}$/.test(trimmed)) {
    return trimmed
  }

  // youtu.be short URLs
  if (trimmed.includes('youtu.be/')) {
    const match = trimmed.match(/youtu\.be\/([^?&/#]+)/)
    return match ? match[1] : null
  }

  // Use comprehensive regex for all other formats
  // Matches: /watch?v=, /embed/, /v/, /shorts/
  const videoIdRegex = /(?:v=|embed\/|v\/|shorts\/)([\w-]{11})/
  const match = trimmed.match(videoIdRegex)

  if (match && match[1]) {
    return match[1]
  }

  // Fallback: try to extract from query parameters
  try {
    const urlObj = new URL(trimmed)
    const vParam = urlObj.searchParams.get('v')
    if (vParam && /^[a-zA-Z0-9_-]{11}$/.test(vParam)) {
      return vParam
    }
  } catch {
    // Not a valid URL, continue
  }

  return null
}

/**
 * Convert YouTube URL or video ID to embed format with playback options
 *
 * Takes any valid YouTube URL format (or just a video ID) and converts it to
 * a clean embed URL with the specified playback options as query parameters.
 *
 * Always extracts the video ID first to ensure clean URL generation without
 * double-transformation issues.
 *
 * @param url - YouTube URL or video ID
 * @param options - Playback options (cc, loop, controls, modestbranding, start)
 * @returns Clean embed URL with options or null if invalid
 */
export function getYoutubeEmbedUrl(url: string, options?: YoutubeEmbedOptions): string | null {
  // Always extract video ID first - this prevents double transformation
  const videoId = getYoutubeVideoId(url)
  if (!videoId) return null

  const params = new URLSearchParams()

  // Add playback options as URL parameters
  if (options?.cc) {
    params.append('cc_load_policy', '1')
  }
  if (options?.loop) {
    params.append('loop', '1')
    params.append('playlist', videoId) // Required for loop to work
  }
  if (options?.controls === false) {
    params.append('controls', '0')
  }
  if (options?.modestbranding) {
    params.append('modestbranding', '1')
  }
  if (options?.start && options.start > 0) {
    params.append('start', options.start.toString())
  }

  const queryString = params.toString()

  // Always build from scratch using the video ID
  return `https://www.youtube.com/embed/${videoId}${queryString ? `?${queryString}` : ''}`
}
