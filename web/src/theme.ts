import { alpha, createTheme, type Theme } from '@mui/material/styles';

/**
 * The portal's design system.
 *
 * Two rules keep it coherent: surfaces are separated by hairline borders rather
 * than drop shadows, and colour is reserved for meaning (status, risk, the
 * active route) instead of decoration. Everything else is neutral, so a red
 * chip in a table actually reads as a warning.
 *
 * Both modes are first-class — this is an operations console people keep open
 * all day.
 */

export type Mode = 'light' | 'dark';

const FONT = '"Inter Variable", "Inter", system-ui, -apple-system, "Segoe UI", sans-serif';
const MONO = '"JetBrains Mono", "SFMono-Regular", ui-monospace, Menlo, Consolas, monospace';

const light = {
  primary: '#0b6bcb',
  primaryDark: '#0a58a8',
  bg: '#f6f8fb',
  paper: '#ffffff',
  // A separate tone for the nav, so the working area is the brightest thing.
  sidebar: '#ffffff',
  text: '#0f172a',
  muted: '#5b6b7f',
  divider: 'rgba(15, 23, 42, 0.10)',
};

const dark = {
  primary: '#5aa2f0',
  primaryDark: '#3d86d6',
  bg: '#0b1120',
  paper: '#121b2c',
  sidebar: '#0e1626',
  text: '#e8eef8',
  muted: '#94a4bb',
  divider: 'rgba(148, 163, 184, 0.18)',
};

export function buildTheme(mode: Mode): Theme {
  const c = mode === 'light' ? light : dark;
  const isLight = mode === 'light';

  const base = createTheme({
    palette: {
      mode,
      primary: { main: c.primary, dark: c.primaryDark },
      secondary: { main: isLight ? '#0d9488' : '#2dd4bf' },
      success: { main: isLight ? '#15803d' : '#4ade80' },
      warning: { main: isLight ? '#b45309' : '#fbbf24' },
      error: { main: isLight ? '#dc2626' : '#f87171' },
      info: { main: isLight ? '#0369a1' : '#38bdf8' },
      background: { default: c.bg, paper: c.paper },
      text: { primary: c.text, secondary: c.muted },
      divider: c.divider,
    },
    shape: { borderRadius: 10 },
    typography: {
      fontFamily: FONT,
      // Headings sit tighter than body text; that contrast is most of what
      // makes an interface feel considered rather than generic.
      h4: { fontWeight: 700, letterSpacing: '-0.022em' },
      h5: { fontWeight: 700, letterSpacing: '-0.018em' },
      h6: { fontWeight: 650, letterSpacing: '-0.012em' },
      subtitle1: { fontWeight: 600 },
      subtitle2: { fontWeight: 600, letterSpacing: '-0.004em' },
      button: { fontWeight: 600, letterSpacing: 0 },
      body2: { lineHeight: 1.6 },
      caption: { letterSpacing: 0 },
    },
  });

  return createTheme(base, {
    components: {
      MuiCssBaseline: {
        styleOverrides: {
          body: {
            backgroundColor: c.bg,
            // Stops long codes and ids from forcing horizontal scroll.
            overflowWrap: 'anywhere',
          },
          code: {
            fontFamily: MONO,
            fontSize: '0.85em',
            padding: '0.1em 0.36em',
            borderRadius: 6,
            backgroundColor: alpha(base.palette.text.primary, isLight ? 0.05 : 0.1),
          },
          '::selection': { backgroundColor: alpha(c.primary, 0.22) },
          '*::-webkit-scrollbar': { width: 10, height: 10 },
          '*::-webkit-scrollbar-thumb': {
            backgroundColor: alpha(base.palette.text.primary, 0.18),
            borderRadius: 8,
            border: `2px solid ${c.bg}`,
          },
          '*::-webkit-scrollbar-thumb:hover': {
            backgroundColor: alpha(base.palette.text.primary, 0.3),
          },
        },
      },

      MuiPaper: {
        styleOverrides: {
          // Elevation is a border here, not a shadow.
          root: { backgroundImage: 'none' },
          outlined: { borderColor: c.divider },
        },
      },

      MuiCard: {
        defaultProps: { elevation: 0 },
        styleOverrides: {
          root: {
            border: `1px solid ${c.divider}`,
            borderRadius: 14,
            backgroundColor: c.paper,
          },
        },
      },

      MuiCardContent: {
        styleOverrides: { root: { padding: 20, '&:last-child': { paddingBottom: 20 } } },
      },

      MuiButton: {
        defaultProps: { disableElevation: true },
        styleOverrides: {
          root: {
            textTransform: 'none',
            borderRadius: 9,
            paddingInline: 16,
            transition: 'background-color .15s ease, border-color .15s ease, color .15s ease',
          },
          sizeSmall: { paddingInline: 12 },
          containedPrimary: {
            '&:hover': { backgroundColor: c.primaryDark },
          },
          outlined: { borderColor: alpha(base.palette.text.primary, 0.22) },
        },
      },

      MuiIconButton: {
        styleOverrides: { root: { borderRadius: 9 } },
      },

      MuiChip: {
        styleOverrides: {
          root: { borderRadius: 7, fontWeight: 600 },
          sizeSmall: { height: 22, fontSize: 12 },
          // Filled status chips read better as soft tints than as solid blocks.
          filledSuccess: {
            backgroundColor: alpha(base.palette.success.main, isLight ? 0.14 : 0.22),
            color: base.palette.success.main,
          },
          filledWarning: {
            backgroundColor: alpha(base.palette.warning.main, isLight ? 0.16 : 0.22),
            color: base.palette.warning.main,
          },
          filledError: {
            backgroundColor: alpha(base.palette.error.main, isLight ? 0.13 : 0.22),
            color: base.palette.error.main,
          },
          filledDefault: {
            backgroundColor: alpha(base.palette.text.primary, isLight ? 0.07 : 0.12),
            color: base.palette.text.secondary,
          },
        },
      },

      MuiOutlinedInput: {
        styleOverrides: {
          root: {
            borderRadius: 9,
            backgroundColor: isLight ? '#fff' : alpha('#ffffff', 0.03),
            '& .MuiOutlinedInput-notchedOutline': { borderColor: c.divider },
            '&:hover .MuiOutlinedInput-notchedOutline': {
              borderColor: alpha(base.palette.text.primary, 0.28),
            },
            '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
              borderColor: c.primary,
              borderWidth: 1.5,
            },
          },
        },
      },

      MuiTableCell: {
        styleOverrides: {
          root: { borderBottom: `1px solid ${c.divider}`, paddingBlock: 12 },
          head: {
            fontWeight: 650,
            fontSize: 12,
            letterSpacing: '0.04em',
            textTransform: 'uppercase',
            color: base.palette.text.secondary,
            backgroundColor: isLight ? alpha('#0f172a', 0.02) : alpha('#ffffff', 0.03),
          },
        },
      },

      MuiTableRow: {
        styleOverrides: {
          root: {
            '&:last-child td': { borderBottom: 0 },
            transition: 'background-color .12s ease',
            '&:hover': { backgroundColor: alpha(base.palette.text.primary, isLight ? 0.025 : 0.04) },
          },
          head: { '&:hover': { backgroundColor: 'transparent' } },
        },
      },

      MuiAlert: {
        styleOverrides: {
          root: { borderRadius: 11, border: `1px solid transparent`, alignItems: 'flex-start' },
          standardInfo: {
            backgroundColor: alpha(base.palette.info.main, isLight ? 0.08 : 0.14),
            borderColor: alpha(base.palette.info.main, 0.25),
          },
          standardWarning: {
            backgroundColor: alpha(base.palette.warning.main, isLight ? 0.1 : 0.14),
            borderColor: alpha(base.palette.warning.main, 0.28),
          },
          standardError: {
            backgroundColor: alpha(base.palette.error.main, isLight ? 0.08 : 0.14),
            borderColor: alpha(base.palette.error.main, 0.26),
          },
          standardSuccess: {
            backgroundColor: alpha(base.palette.success.main, isLight ? 0.1 : 0.14),
            borderColor: alpha(base.palette.success.main, 0.26),
          },
        },
      },

      MuiTooltip: {
        styleOverrides: {
          tooltip: {
            backgroundColor: isLight ? '#0f172a' : '#1e293b',
            border: `1px solid ${alpha('#ffffff', 0.1)}`,
            borderRadius: 10,
            fontSize: 12.5,
            padding: '10px 12px',
            boxShadow: '0 8px 28px rgba(2, 6, 23, 0.28)',
          },
          arrow: { color: isLight ? '#0f172a' : '#1e293b' },
        },
      },

      MuiDrawer: {
        styleOverrides: {
          paper: { backgroundColor: c.sidebar, borderRight: `1px solid ${c.divider}` },
        },
      },

      MuiAppBar: {
        defaultProps: { elevation: 0, color: 'inherit' },
        styleOverrides: {
          root: {
            // Translucent, so content scrolling underneath stays perceptible.
            backgroundColor: alpha(c.paper, isLight ? 0.82 : 0.72),
            backdropFilter: 'saturate(180%) blur(12px)',
            borderBottom: `1px solid ${c.divider}`,
            color: base.palette.text.primary,
          },
        },
      },

      MuiMenu: {
        styleOverrides: {
          paper: {
            borderRadius: 12,
            border: `1px solid ${c.divider}`,
            boxShadow: isLight
              ? '0 10px 34px rgba(15, 23, 42, 0.12)'
              : '0 10px 34px rgba(0, 0, 0, 0.5)',
            marginTop: 6,
          },
        },
      },

      MuiMenuItem: {
        styleOverrides: { root: { borderRadius: 8, marginInline: 6, minHeight: 38 } },
      },

      MuiListItemButton: {
        styleOverrides: {
          root: {
            borderRadius: 10,
            '&.Mui-selected': {
              backgroundColor: alpha(c.primary, isLight ? 0.1 : 0.18),
              color: c.primary,
              '&:hover': { backgroundColor: alpha(c.primary, isLight ? 0.14 : 0.24) },
              '& .MuiListItemIcon-root': { color: c.primary },
            },
          },
        },
      },

      MuiListItemIcon: { styleOverrides: { root: { minWidth: 38 } } },

      MuiDivider: { styleOverrides: { root: { borderColor: c.divider } } },

      MuiTabs: {
        styleOverrides: { indicator: { height: 3, borderRadius: 3 } },
      },
      MuiTab: {
        styleOverrides: { root: { textTransform: 'none', fontWeight: 600, minHeight: 44 } },
      },

      MuiLink: { defaultProps: { underline: 'hover' } },

      MuiCheckbox: { defaultProps: { disableRipple: false } },
    },
  });
}
