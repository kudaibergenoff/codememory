// User sessions. Sessions expire after a period of inactivity.
const IDLE_LIMIT_MS = 30 * 60 * 1000;

export interface Session {
  userId: string;
  lastSeen: number;
}

export function createSession(userId: string): Session {
  return { userId, lastSeen: Date.now() };
}

export function isExpired(session: Session, now: number): boolean {
  return now - session.lastSeen > IDLE_LIMIT_MS;
}
