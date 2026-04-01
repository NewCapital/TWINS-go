/**
 * Logger utility for conditional logging based on environment
 */

export enum LogLevel {
  DEBUG = 'DEBUG',
  INFO = 'INFO',
  WARN = 'WARN',
  ERROR = 'ERROR',
}

class Logger {
  private isDevelopment: boolean;

  constructor() {
    this.isDevelopment = import.meta.env.DEV;
  }

  debug(...args: any[]): void {
    if (this.isDevelopment) {
      console.log('[DEBUG]', ...args);
    }
  }

  info(...args: any[]): void {
    if (this.isDevelopment) {
      console.log('[INFO]', ...args);
    }
  }

  warn(...args: any[]): void {
    console.warn('[WARN]', ...args);
  }

  error(...args: any[]): void {
    console.error('[ERROR]', ...args);
  }

  // For explicitly logging in production (use sparingly)
  force(...args: any[]): void {
    console.log(...args);
  }
}

export const logger = new Logger();
