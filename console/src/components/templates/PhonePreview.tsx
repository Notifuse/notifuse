import React from 'react'
import { useLingui } from '@lingui/react/macro'

// Import specific Lucide icons
import { Signal, Wifi, BatteryFull, ChevronRight } from 'lucide-react'

// Define Styles as JavaScript Objects with proper TypeScript typing
const styles: Record<string, React.CSSProperties> = {
  iphoneMockupUpper: {
    width: '375px',
    height: '350px',
    border: '12px solid black',
    borderBottom: 'none',
    borderTopLeftRadius: '50px',
    borderTopRightRadius: '50px',
    backgroundColor: '#f8f8f8',
    position: 'relative',
    boxShadow: '0 10px 30px rgba(0, 0, 0, 0.2)',
    overflow: 'hidden',
    fontFamily:
      '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif',
    margin: '0px auto' // Center horizontally for demo
  },
  screen: {
    width: '100%',
    height: '100%',
    backgroundColor: '#ffffff',
    borderTopLeftRadius: '38px',
    borderTopRightRadius: '38px',
    overflow: 'hidden',
    position: 'relative',
    display: 'flex',
    flexDirection: 'column'
  },
  dynamicIsland: {
    position: 'absolute',
    top: '10px',
    left: '50%',
    transform: 'translateX(-50%)',
    width: '125px',
    height: '36px',
    backgroundColor: 'black',
    borderRadius: '18px',
    zIndex: 10
  },
  statusBar: {
    position: 'absolute',
    top: '16px',
    left: '0',
    right: '0',
    height: '24px',
    padding: '0 25px',
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    zIndex: 5,
    color: '#000'
  },
  statusBarLeft: {
    // No specific styles needed beyond container
  },
  time: {
    fontSize: '14px',
    fontWeight: 600 // Numbers are valid for fontWeight
  },
  statusBarRight: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    fontSize: '13px' // Applied via icon props where possible
  },
  emailPreview: {
    paddingTop: '5px',
    paddingLeft: '15px',
    paddingRight: '15px',
    paddingBottom: '15px',
    flexGrow: 1
  },
  inboxHeader: {
    fontSize: '22px',
    fontWeight: 700,
    marginTop: '65px',
    marginBottom: '15px',
    paddingBottom: '8px',
    borderBottom: '1px solid #e0e0e0',
    paddingLeft: '15px',
    paddingRight: '15px'
  },
  emailHeader: {
    display: 'flex',
    alignItems: 'flex-start',
    justifyContent: 'space-between'
  },
  unreadDot: {
    width: '10px',
    height: '10px',
    borderRadius: '50%',
    backgroundColor: '#007AFF',
    marginRight: '10px',
    marginTop: '6px',
    flexShrink: 0
  },
  emailContent: {
    display: 'flex',
    alignItems: 'flex-start',
    flexGrow: 1
  },
  emailDetails: {
    flexGrow: 1,
    overflow: 'hidden'
  },
  sender: {
    fontSize: '16px',
    fontWeight: 600,
    marginBottom: '2px',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis'
  },
  subject: {
    fontSize: '15px',
    fontWeight: 500,
    marginBottom: '3px',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis'
  },
  previewText: {
    fontSize: '14px',
    color: '#666',
    lineHeight: 1.3, // unitless line-height is valid
    display: '-webkit-box',
    WebkitLineClamp: 2, // Use camelCase for vendor prefixes
    WebkitBoxOrient: 'vertical',
    overflow: 'hidden',
    wordBreak: 'break-word'
  },
  emailMeta: {
    flexShrink: 0,
    paddingLeft: '5px'
  },
  timestamp: {
    fontSize: '13px',
    color: '#8e8e93',
    whiteSpace: 'nowrap',
    display: 'flex',
    alignItems: 'center'
  },
  statusIcon: {
    display: 'inline-block',
    width: 16,
    height: 16
  },
  chevronIcon: {
    display: 'inline-block',
    width: 12,
    height: 12,
    marginLeft: '5px',
    color: '#c7c7cc',
    strokeWidth: 2.5
  }
}

// --- React Component ---

const IphoneEmailPreview = ({
  sender,
  subject,
  previewText,
  timestamp,
  currentTime = '9:41'
}: {
  sender: string
  subject: string
  previewText: string
  timestamp: string
  currentTime: string
}) => {
  const { t } = useLingui()

  return (
    <div style={styles.iphoneMockupUpper}>
      <div style={styles.screen}>
        {/* Dynamic Island */}
        <div style={styles.dynamicIsland}></div>

        {/* Status Bar Elements */}
        <div style={styles.statusBar}>
          <div style={styles.statusBarLeft}>
            <span style={styles.time}>{currentTime}</span>
          </div>
          <div style={styles.statusBarRight}>
            {/* Use Lucide components with style props */}
            <Signal style={styles.statusIcon} size={16} />
            <Wifi style={styles.statusIcon} size={16} />
            <BatteryFull style={styles.statusIcon} size={16} />
          </div>
        </div>

        {/* Inbox Header */}
        <div style={styles.inboxHeader}>{t`Inbox`}</div>

        {/* Email Preview Content */}
        <div style={styles.emailPreview}>
          <div style={styles.emailHeader}>
            <div style={styles.emailContent}>
              <div style={styles.unreadDot}></div>
              <div style={styles.emailDetails}>
                <div style={styles.sender}>{sender}</div>
                <div style={styles.subject}>{subject}</div>
                <div style={styles.previewText}>{previewText}</div>
              </div>
            </div>
            <div style={styles.emailMeta}>
              <div style={styles.timestamp}>
                {timestamp}
                <ChevronRight style={styles.chevronIcon} />
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

export default IphoneEmailPreview
