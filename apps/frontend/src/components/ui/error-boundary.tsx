import { Component, type ErrorInfo, type ReactNode } from 'react';

interface ErrorBoundaryProps {
  children: ReactNode;
  fallback: ReactNode;
}

interface ErrorBoundaryState {
  hasError: boolean;
}

/**
 * ErrorBoundary catches render-time errors in its subtree (e.g. a WebGL context
 * loss or a Three.js exception inside the R3F Canvas) and shows a fallback
 * instead of letting the error unmount the whole SPA. React has no hook-based
 * equivalent, so this stays a class component by necessity.
 */
export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(): ErrorBoundaryState {
    return { hasError: true };
  }

  override componentDidCatch(error: Error, info: ErrorInfo): void {
    // Local-first diagnostics tool: surface the error in the console for debugging.
    console.error('ErrorBoundary caught an error', error, info);
  }

  override render(): ReactNode {
    if (this.state.hasError) {
      return this.props.fallback;
    }
    return this.props.children;
  }
}
