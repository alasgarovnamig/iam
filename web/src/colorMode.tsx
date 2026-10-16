import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { CssBaseline, ThemeProvider } from '@mui/material';
import { buildTheme, type Mode } from './theme';
import { ColorModeContext, type ColorModeValue } from './colorModeContext';

/**
 * Light/dark preference. Starts from the operating system, and once the user
 * picks a side that choice wins and is remembered.
 *
 * Only the preference is persisted — never anything about the session.
 */

const STORAGE_KEY = 'wg.color-mode';

function initialMode(): Mode {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === 'light' || stored === 'dark') return stored;
  } catch {
    // Private mode or storage disabled — fall through to the system setting.
  }
  if (typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: dark)').matches) {
    return 'dark';
  }
  return 'light';
}

export function ColorModeProvider({ children }: { children: ReactNode }) {
  const [mode, setMode] = useState<Mode>(initialMode);

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, mode);
    } catch {
      // Not being able to remember the choice is not worth failing over.
    }
    document.documentElement.style.colorScheme = mode;
  }, [mode]);

  const value = useMemo<ColorModeValue>(
    () => ({ mode, toggle: () => setMode((m) => (m === 'light' ? 'dark' : 'light')) }),
    [mode],
  );

  const theme = useMemo(() => buildTheme(mode), [mode]);

  return (
    <ColorModeContext.Provider value={value}>
      <ThemeProvider theme={theme}>
        <CssBaseline />
        {children}
      </ThemeProvider>
    </ColorModeContext.Provider>
  );
}
