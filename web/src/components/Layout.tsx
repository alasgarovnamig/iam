import { useState, type ReactNode } from 'react';
import { Link as RouterLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import {
  AppBar,
  Avatar,
  Box,
  Chip,
  Divider,
  Drawer,
  IconButton,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  Stack,
  Toolbar,
  Tooltip,
  Typography,
  alpha,
} from '@mui/material';
import ApartmentIcon from '@mui/icons-material/Apartment';
import PeopleIcon from '@mui/icons-material/People';
import SecurityIcon from '@mui/icons-material/Security';
import VpnKeyIcon from '@mui/icons-material/VpnKey';
import HistoryIcon from '@mui/icons-material/History';
import DashboardIcon from '@mui/icons-material/Dashboard';
import DevicesIcon from '@mui/icons-material/Devices';
import LightModeIcon from '@mui/icons-material/LightModeOutlined';
import DarkModeIcon from '@mui/icons-material/DarkModeOutlined';
import LogoutIcon from '@mui/icons-material/Logout';
import PersonOutlineIcon from '@mui/icons-material/PersonOutline';
import { useAuth } from '../auth/AuthContext';
import { useColorMode } from '../colorModeContext';
import { P } from '../auth/permissions';
import { displayName } from '../api/types';

const drawerWidth = 252;

interface NavItem {
  label: string;
  to: string;
  icon: ReactNode;
  /** The item is hidden unless the actor holds one of these. */
  anyOf?: string[];
}

const navItems: NavItem[] = [
  { label: 'Dashboard', to: '/', icon: <DashboardIcon /> },
  { label: 'Tenants', to: '/tenants', icon: <ApartmentIcon />, anyOf: [P.tenants.list, P.tenants.read] },
  { label: 'Users', to: '/users', icon: <PeopleIcon />, anyOf: [P.users.list, P.users.read] },
  { label: 'Roles', to: '/roles', icon: <SecurityIcon />, anyOf: [P.roles.list, P.roles.read] },
  { label: 'API Clients', to: '/api-clients', icon: <VpnKeyIcon />, anyOf: [P.apiClients.list, P.apiClients.read] },
  { label: 'Audit Logs', to: '/audit-logs', icon: <HistoryIcon />, anyOf: [P.auditLogs.list, P.auditLogs.read] },
  { label: 'Sessions', to: '/sessions', icon: <DevicesIcon /> },
];

/** Two letters from the person's name, for the avatar. */
function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return '?';
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

export function Layout() {
  const { user, permissions, logout } = useAuth();
  const { mode, toggle } = useColorMode();
  const location = useLocation();
  const navigate = useNavigate();
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);

  // Menu entries the actor has no permission for are not rendered at all.
  const visibleItems = navItems.filter((item) => !item.anyOf || permissions.hasAny(...item.anyOf));
  const isActive = (to: string) =>
    to === '/' ? location.pathname === '/' : location.pathname.startsWith(to);
  const current = visibleItems.find((i) => isActive(i.to));

  const handleLogout = async () => {
    setAnchorEl(null);
    await logout();
    navigate('/login', { replace: true });
  };

  const name = user ? displayName(user) : '';
  const isPlatform = user?.user_type === 'platform_user';

  return (
    <Box sx={{ display: 'flex' }}>
      <AppBar position="fixed" sx={{ zIndex: (t) => t.zIndex.drawer + 1 }}>
        <Toolbar sx={{ gap: 1 }}>
          <Stack direction="row" alignItems="center" spacing={1.25} sx={{ width: drawerWidth - 24 }}>
            <Box
              aria-hidden
              sx={{
                width: 30,
                height: 30,
                borderRadius: 2,
                display: 'grid',
                placeItems: 'center',
                color: '#fff',
                fontWeight: 800,
                fontSize: 15,
                background: (t) =>
                  `linear-gradient(135deg, ${t.palette.primary.main}, ${t.palette.secondary.main})`,
              }}
            >
              W
            </Box>
            <Typography variant="h6" component="div" sx={{ fontWeight: 700, letterSpacing: '-0.02em' }}>
              Wallet Gateway
            </Typography>
          </Stack>

          {/* Where you are, so the title is not only in the sidebar. */}
          <Typography
            variant="subtitle2"
            color="text.secondary"
            sx={{ flexGrow: 1, display: { xs: 'none', sm: 'block' } }}
          >
            {current?.label ?? ''}
          </Typography>

          <Tooltip title={mode === 'light' ? 'Switch to dark' : 'Switch to light'} arrow>
            <IconButton onClick={toggle} aria-label="toggle colour mode" data-testid="toggle-color-mode">
              {mode === 'light' ? <DarkModeIcon fontSize="small" /> : <LightModeIcon fontSize="small" />}
            </IconButton>
          </Tooltip>

          <Stack direction="row" alignItems="center" spacing={1.25} sx={{ ml: 0.5 }}>
            <Box sx={{ textAlign: 'right', display: { xs: 'none', md: 'block' }, lineHeight: 1.2 }}>
              <Typography variant="body2" fontWeight={600} data-testid="current-user">
                {name}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                {isPlatform ? 'Platform' : 'Tenant'}
              </Typography>
            </Box>
            <IconButton
              onClick={(e) => setAnchorEl(e.currentTarget)}
              aria-label="account menu"
              sx={{ p: 0.5 }}
            >
              <Avatar
                sx={{
                  width: 34,
                  height: 34,
                  fontSize: 13,
                  fontWeight: 700,
                  bgcolor: (t) => alpha(t.palette.primary.main, 0.16),
                  color: 'primary.main',
                }}
              >
                {initials(name)}
              </Avatar>
            </IconButton>
          </Stack>

          <Menu
            anchorEl={anchorEl}
            open={Boolean(anchorEl)}
            onClose={() => setAnchorEl(null)}
            transformOrigin={{ horizontal: 'right', vertical: 'top' }}
            anchorOrigin={{ horizontal: 'right', vertical: 'bottom' }}
            slotProps={{ paper: { sx: { minWidth: 210 } } }}
          >
            <Box sx={{ px: 2, py: 1 }}>
              <Typography variant="body2" fontWeight={600} noWrap>
                {name}
              </Typography>
              <Typography variant="caption" color="text.secondary" noWrap>
                {user?.email ?? ''}
              </Typography>
            </Box>
            <Divider sx={{ my: 0.5 }} />
            <MenuItem
              component={RouterLink}
              to="/profile"
              onClick={() => setAnchorEl(null)}
              data-testid="menu-profile"
            >
              <ListItemIcon>
                <PersonOutlineIcon fontSize="small" />
              </ListItemIcon>
              Profile
            </MenuItem>
            <MenuItem onClick={handleLogout} data-testid="menu-logout">
              <ListItemIcon>
                <LogoutIcon fontSize="small" />
              </ListItemIcon>
              Sign out
            </MenuItem>
          </Menu>
        </Toolbar>
      </AppBar>

      <Drawer
        variant="permanent"
        sx={{
          width: drawerWidth,
          flexShrink: 0,
          [`& .MuiDrawer-paper`]: { width: drawerWidth, boxSizing: 'border-box' },
        }}
      >
        <Toolbar />
        <Box sx={{ overflow: 'auto', px: 1.5, py: 2 }}>
          <Typography
            variant="caption"
            sx={{
              px: 1.5,
              color: 'text.secondary',
              fontWeight: 700,
              letterSpacing: '0.08em',
              textTransform: 'uppercase',
            }}
          >
            Manage
          </Typography>
          <List sx={{ mt: 0.5, display: 'flex', flexDirection: 'column', gap: 0.25 }}>
            {visibleItems.map((item) => {
              const active = isActive(item.to);
              return (
                <ListItemButton
                  key={item.to}
                  component={RouterLink}
                  to={item.to}
                  selected={active}
                  data-testid={`nav-${item.label.toLowerCase().replace(/\s+/g, '-')}`}
                  sx={{
                    position: 'relative',
                    // A left rail on the active item — quicker to scan than
                    // background colour alone.
                    '&::before': active
                      ? {
                          content: '""',
                          position: 'absolute',
                          left: 0,
                          top: 8,
                          bottom: 8,
                          width: 3,
                          borderRadius: 3,
                          backgroundColor: 'primary.main',
                        }
                      : undefined,
                  }}
                >
                  <ListItemIcon>{item.icon}</ListItemIcon>
                  <ListItemText
                    primary={item.label}
                    primaryTypographyProps={{ fontSize: 14.5, fontWeight: active ? 650 : 500 }}
                  />
                </ListItemButton>
              );
            })}
          </List>
        </Box>

        <Box sx={{ mt: 'auto', p: 2 }}>
          <Chip
            size="small"
            label={isPlatform ? 'Platform scope' : 'Tenant scope'}
            color={isPlatform ? 'primary' : 'default'}
            variant="outlined"
            sx={{ width: '100%' }}
          />
        </Box>
      </Drawer>

      <Box
        component="main"
        sx={{ flexGrow: 1, minHeight: '100vh', bgcolor: 'background.default', minWidth: 0 }}
      >
        <Toolbar />
        <Box sx={{ p: { xs: 2, md: 3 }, maxWidth: 1440, mx: 'auto' }}>
          <Outlet />
        </Box>
      </Box>
    </Box>
  );
}
