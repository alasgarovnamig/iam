import { createContext, useContext } from 'react';
import type { Mode } from './theme';

/**
 * The light/dark preference, kept separate from the provider component so
 * hot reload can still swap the provider without dropping the context.
 */
export interface ColorModeValue {
  mode: Mode;
  toggle: () => void;
}

export const ColorModeContext = createContext<ColorModeValue>({
  mode: 'light',
  toggle: () => {},
});

export function useColorMode(): ColorModeValue {
  return useContext(ColorModeContext);
}
