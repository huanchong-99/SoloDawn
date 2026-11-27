// The fixed ids of the demo world: groups, members and computers, the same on every start, so that
// a browser that paired with the world finds it again after a reload. Each is derived once from a
// name with SHA-256 (16 bytes for a random id, 32 for a device id) and written out here.
import { owner } from '../home';

export const ids = {
  site: 'BsebuFCdYvLaA9AbRCg9dA',
  talk: 'sAMBSgZyIJo0pKEACZb8tA',
  blog: 'VpKGCCq3zA0bPrDeyF7C_A',
  /** The person at this browser, in every group: the member id the test homes write for them. */
  me: owner,
  secretary: 'geb2UHpWAhcQHK7NLhk4Og',
  claude: '5l9SNZ1VMyXF0a3Sq3Ik4g',
  codex: 'nssBhjqOr_QUjZFlhuZ-MA',
  mina: 'ybVgSvDrNjzeUp1A5jvMVg',
  minaAi: 'ClIbmHyJqjLSWCFIrynHQg',
  minaTalk: '7doctR3yhr0PKuGiENYJ0Q',
  leo: 'ZTOztQLrN05IQixJLCmskA',
  noraBlog: 'PVsxxaFQ4UPcrTBctiLd1A',
};

export const devices = {
  macbook: 'VF4OWlN-6HJ2P8fiGMDohEEk9GO25IG3fVmrtyTlPOA',
  office: 'Xqh1oTwBoVAHF8kWloU5uWY9By2PmrGtlUw7-WcvbdE',
  minaPc: 'D6y85Pgbn2sfSIV83Q4RXMsUkOMBoxJFaA2Wr3wh_CI',
};

/** The people other than the one at this browser, by their user id. */
export const users = {
  mina: 'PdL7iYPxf1msb6CLlsHUkZJTtnPlzflgYNoYZNcY-F4',
  leo: 'WIDKvh-CYfCLhlVbSUXKT8QCkNq_ZCOKwZqFh7mZNMw',
  nora: 'sORopNDxmIOr5XQyYnyWUFfzf4XkHtfagoEefbRzudo',
};
