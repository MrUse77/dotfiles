-- #######################################################################################
-- HYPRLAND LUA CONFIG
-- Migrated from hyprland.conf — Hyprland 0.55+
-- Refer to https://wiki.hypr.land/Configuring/Start/
-- #######################################################################################

-- Please note not all available settings / options are set here.
-- For a full list, see the wiki

-- You can split this configuration into multiple files
-- Create your files separately and then require them like this:
-- require("myColors")


------------------
---- MONITORS ----
------------------

-- See https://wiki.hypr.land/Configuring/Basics/Monitors/
hl.monitor({
    output   = "",
    mode     = "preferred",
    position = "auto",
    scale    = 1,
})

-- Workspaces are auto-created; explicit declarations are optional in Lua.
-- We keep them here for clarity.
hl.workspace_rule({ workspace = "1" })
hl.workspace_rule({ workspace = "2" })
hl.workspace_rule({ workspace = "3" })
hl.workspace_rule({ workspace = "4" })
hl.workspace_rule({ workspace = "5" })


---------------------
---- MY PROGRAMS ----
---------------------

-- See https://wiki.hypr.land/Configuring/Basics/Variables/

-- Set programs that you use
local terminal    = "ghostty"
local fileManager = "thunar"
local menu        = "nwg-drawer -c 6 -is 64 -ovl -nofs"


-------------------
---- AUTOSTART ----
-------------------

-- Autostart necessary processes (like notifications daemons, status bars, etc.)
-- Or execute your favorite apps at launch like this:

hl.on("hyprland.start", function()
    hl.exec_cmd("dbus-update-activation-environment --systemd WAYLAND_DISPLAY XDG_CURRENT_DESKTOP")
    hl.exec_cmd("/usr/lib/hyprpolkitagent/hyprpolkitagent & waybar & hyprctl setcursor volantes_cursors 24 & dunst & hyprsunset")
    hl.exec_cmd("hyprpm reload -n & hyprpaper & corectrl & /usr/lib/xdg-desktop-portal-hyprland")
end)


-------------------------------
---- ENVIRONMENT VARIABLES ----
-------------------------------

-- See https://wiki.hypr.land/Configuring/Advanced-and-Cool/Environment-variables/

hl.env("XCURSOR_SIZE", "24")
hl.env("HYPRCURSOR_SIZE", "24")
hl.env("XCURSOR_THEME", "volantes_cursors")

-- --- TOOLKITS BACKENDS ---
-- Forzar a GTK a usar Wayland, fallback a X11 si falla
hl.env("GDK_BACKEND", "wayland,x11,*")
-- Forzar a Qt a usar Wayland
hl.env("QT_QPA_PLATFORM", "wayland;xcb")
-- Forzar SDL (Juegos) a usar Wayland
hl.env("SDL_VIDEODRIVER", "wayland")
-- Forzar Clutter a usar Wayland
hl.env("CLUTTER_BACKEND", "wayland")

-- --- QT SPECIFICS ---
hl.env("QT_QPA_PLATFORMTHEME", "qt5ct")
hl.env("QT_WAYLAND_DISABLE_WINDOWDECORATION", "1")
hl.env("QT_AUTO_SCREEN_SCALE_FACTOR", "1")
hl.env("PATH", os.getenv("PATH") .. ":" .. os.getenv("HOME") .. "/.nix-profile/bin")

-- --- ELECTRON / CHROMIUM ---
-- Crítico para que VS Code, Discord, Chrome usen Wayland nativo
hl.env("ELECTRON_OZONE_PLATFORM_HINT", "auto")

hl.env("XDG_CURRENT_DESKTOP", "Hyprland")
hl.env("XDG_SESSION_TYPE", "wayland")
hl.env("XDG_SESSION_DESKTOP", "Hyprland")


-----------------------
----- PERMISSIONS -----
-----------------------

-- See https://wiki.hypr.land/Configuring/Advanced-and-Cool/Permissions/
-- Please note permission changes here require a Hyprland restart and are not applied on-the-fly
-- for security reasons

-- hl.config({
--   ecosystem = {
--     enforce_permissions = true,
--   },
-- })

-- hl.permission("/usr/(bin|local/bin)/grim", "screencopy", "allow")
-- hl.permission("/usr/(lib|libexec|lib64)/xdg-desktop-portal-hyprland", "screencopy", "allow")
-- hl.permission("/usr/(bin|local/bin)/hyprpm", "plugin", "allow")


-----------------------
---- LOOK AND FEEL ----
-----------------------

-- Refer to https://wiki.hypr.land/Configuring/Basics/Variables/

hl.config({
    general = {
        gaps_in  = 10,
        gaps_out = 20,

        border_size = 5,

        -- https://wiki.hypr.land/Configuring/Basics/Variables/#variable-types for info about colors
        col = {
            active_border   = "rgba(262738aa)",
            inactive_border = "rgba(595959aa)",
        },

        -- Set to true to enable resizing windows by clicking and dragging on borders and gaps
        resize_on_border = true,
        extend_border_grab_area = 0,

        -- Please see https://wiki.hypr.land/Configuring/Advanced-and-Cool/Tearing/ before you turn this on
        allow_tearing = true,

        layout = "dwindle",
    },

    decoration = {
        rounding       = 15,
        rounding_power = 10,

        -- Change transparency of focused and unfocused windows
        active_opacity   = 0.9,
        inactive_opacity = 0.5,

        shadow = {
            enabled      = true,
            range        = 5,
            render_power = 2,
            color        = 0xee7aa2f7, -- rgba(122, 162, 247, 1) -> 0xee + RGB
        },

        -- https://wiki.hypr.land/Configuring/Advanced-and-Cool/Blur/
        blur = {
            enabled          = true,
            size             = 4,
            passes           = 4,
            new_optimizations = true,
            vibrancy         = 0.1696,
        },
    },

    -- https://wiki.hypr.land/Configuring/Advanced-and-Cool/Animations/
    animations = {
        enabled = true,
    },
})

-- MoonArch theme colors override these defaults when available.
-- The Lua fragment returns a partial config table containing general.col.
local theme_path = os.getenv("HOME") .. "/.local/share/moonarch/themes/current/hyprland.lua"
local theme_file = io.open(theme_path, "r")
if theme_file then
    theme_file:close()
    local loaded, theme_config = pcall(dofile, theme_path)
    if loaded and type(theme_config) == "table" then
        hl.config(theme_config)
    end
end

-- Default animation curves, see https://wiki.hypr.land/Configuring/Advanced-and-Cool/Animations/
hl.curve("easeOutQuint",   { type = "bezier", points = { {0.23, 1},    {0.32, 1}    } })
hl.curve("easeInOutCubic", { type = "bezier", points = { {0.65, 0.05}, {0.36, 1}    } })
hl.curve("linear",         { type = "bezier", points = { {0, 0},       {1, 1}       } })
hl.curve("almostLinear",   { type = "bezier", points = { {0.5, 0.5},   {0.75, 1}    } })
hl.curve("quick",          { type = "bezier", points = { {0.15, 0},    {0.1, 1}     } })

-- Animation definitions
hl.animation({ leaf = "global",        enabled = true, speed = 10,   bezier = "default" })
hl.animation({ leaf = "border",        enabled = true, speed = 5.39, bezier = "easeOutQuint" })
hl.animation({ leaf = "windows",       enabled = true, speed = 4.79, bezier = "easeOutQuint" })
hl.animation({ leaf = "windowsIn",     enabled = true, speed = 4.1,  bezier = "easeOutQuint", style = "popin 87%" })
hl.animation({ leaf = "windowsOut",    enabled = true, speed = 1.49, bezier = "linear",       style = "popin 87%" })
hl.animation({ leaf = "fadeIn",        enabled = true, speed = 1.73, bezier = "almostLinear" })
hl.animation({ leaf = "fadeOut",       enabled = true, speed = 1.46, bezier = "almostLinear" })
hl.animation({ leaf = "fade",          enabled = true, speed = 3.03, bezier = "quick" })
hl.animation({ leaf = "layers",        enabled = true, speed = 3.81, bezier = "easeOutQuint" })
hl.animation({ leaf = "layersIn",      enabled = true, speed = 4,    bezier = "easeOutQuint", style = "fade" })
hl.animation({ leaf = "layersOut",     enabled = true, speed = 1.5,  bezier = "linear",       style = "fade" })
hl.animation({ leaf = "fadeLayersIn",  enabled = true, speed = 1.79, bezier = "almostLinear" })
hl.animation({ leaf = "fadeLayersOut", enabled = true, speed = 1.39, bezier = "almostLinear" })
hl.animation({ leaf = "workspaces",    enabled = true, speed = 1.94, bezier = "almostLinear", style = "fade" })
hl.animation({ leaf = "workspacesIn",  enabled = true, speed = 1.21, bezier = "almostLinear", style = "fade" })
hl.animation({ leaf = "workspacesOut", enabled = true, speed = 1.94, bezier = "almostLinear", style = "fade" })

-- Ref https://wiki.hypr.land/Configuring/Basics/Workspace-Rules/
-- "Smart gaps" / "No gaps when only"
-- uncomment all if you wish to use that.
-- hl.workspace_rule({ workspace = "w[tv1]", gaps_out = 0, gaps_in = 0 })
-- hl.workspace_rule({ workspace = "f[1]",   gaps_out = 0, gaps_in = 0 })
-- hl.window_rule({
--     name  = "no-gaps-wtv1",
--     match = { float = false, workspace = "w[tv1]" },
--     border_size = 0,
--     rounding    = 0,
-- })
-- hl.window_rule({
--     name  = "no-gaps-f1",
--     match = { float = false, workspace = "f[1]" },
--     border_size = 0,
--     rounding    = 0,
-- })

-- See https://wiki.hypr.land/Configuring/Layouts/Dwindle-Layout/ for more
hl.config({
    dwindle = {
        -- pseudotile = true, -- Master switch for pseudotiling. Enabling is bound to mainMod + P below
        preserve_split = true, -- You probably want this
    },
})

-- See https://wiki.hypr.land/Configuring/Layouts/Master-Layout/ for more
hl.config({
    master = {
        new_status = "master",
    },
})


----------------
----  MISC  ----
----------------

hl.config({
    misc = {
        force_default_wallpaper = 0,   -- Set to 0 or 1 to disable the anime mascot wallpapers
        disable_hyprland_logo   = false, -- If true disables the random hyprland logo / anime girl background. :(
    },
    render = {
        direct_scanout = false,
    },
})


---------------
---- INPUT ----
---------------

hl.config({
    input = {
        kb_layout  = "us,latam",
        kb_variant = "",
        kb_model   = "",
        kb_options = "",
        kb_rules   = "",

        follow_mouse = 1,

        sensitivity    = 0,    -- -1.0 - 1.0, 0 means no modification.
        accel_profile  = "flat",

        touchpad = {
            natural_scroll = false,
        },
    },
})

-- Gestures
hl.gesture({
    fingers   = 3,
    direction = "horizontal",
    action    = "workspace",
})
hl.gesture({
    fingers   = 4,
    direction = "down",
    mod       = "ALT",
    action    = "close",
})
hl.gesture({
    fingers   = 3,
    direction = "up",
    mod       = "SUPER",
    action    = "scale:1.5,fullscreen",
})

-- Example per-device config
-- See https://wiki.hypr.land/Configuring/Advanced-and-Cool/Devices/ for more


---------------------
---- KEYBINDINGS ----
---------------------

-- See https://wiki.hypr.land/Configuring/Basics/Binds/
local mainMod = "SUPER" -- Sets "Windows" key as main modifier

-- Example binds, see https://wiki.hypr.land/Configuring/Basics/Binds/ for more
hl.bind(mainMod .. " + Return",         hl.dsp.exec_cmd(terminal))
hl.bind(mainMod .. " + Q",              hl.dsp.window.close())
hl.bind(mainMod .. " + SHIFT + Q",      hl.dsp.exec_cmd("exit"))
hl.bind(mainMod .. " + E",              hl.dsp.exec_cmd(fileManager))
hl.bind(mainMod .. " + V",              hl.dsp.window.float({ action = "toggle" }))
hl.bind(mainMod .. " + M",              hl.dsp.exec_cmd(menu))
hl.bind(mainMod .. " + P",              hl.dsp.window.pseudo()) -- dwindle
-- hl.bind(mainMod .. " + J",           hl.dsp.layout("togglesplit")) -- dwindle
hl.bind(mainMod .. " + F",              hl.dsp.window.fullscreen())
-- hl.bind(mainMod .. " + T",           hl.dsp.layout("togglesplit"))
hl.bind(mainMod .. " + Space",          hl.dsp.exec_cmd("hyprctl switchxkblayout current next"))
hl.bind("PRINT",                        hl.dsp.exec_cmd("hyprshot -m region --clipboard-only"))
hl.bind("SUPER + SHIFT + L",            hl.dsp.exec_cmd("hyprlock"))
hl.bind(mainMod .. " + Y",              hl.dsp.exec_cmd("ghostty --config-file=" .. os.getenv("HOME") .. "/.config/ghostty/config-clean -e yazi"))
hl.bind("SUPER + SHIFT + Y",            hl.dsp.exec_cmd("ghostty --config-file=" .. os.getenv("HOME") .. "/.config/ghostty/config-clean -e sudo yazi"))

-- MoonArch theme selector — phase 2 will wire this properly
hl.bind(mainMod .. " + SHIFT + T",      hl.dsp.exec_cmd(os.getenv("HOME") .. "/.local/bin/moonarch/theme-selector"))


-- Move focus with mainMod + arrow keys
hl.bind(mainMod .. " + left",  hl.dsp.focus({ direction = "left" }))
hl.bind(mainMod .. " + right", hl.dsp.focus({ direction = "right" }))
hl.bind(mainMod .. " + up",    hl.dsp.focus({ direction = "up" }))
hl.bind(mainMod .. " + down",  hl.dsp.focus({ direction = "down" }))

-- Switch workspaces with mainMod + [0-9]
-- Move active window to a workspace with mainMod + SHIFT + [0-9]
for i = 1, 10 do
    local key = i % 10 -- 10 maps to key 0
    hl.bind(mainMod .. " + " .. key,             hl.dsp.focus({ workspace = i }))
    hl.bind(mainMod .. " + SHIFT + " .. key,     hl.dsp.window.move({ workspace = i }))
end

-- Additional workspace navigation
hl.bind(mainMod .. " + h", hl.dsp.focus({ workspace = "e-1" }))
hl.bind(mainMod .. " + j", hl.dsp.focus({ workspace = "e-5" }))
hl.bind(mainMod .. " + k", hl.dsp.focus({ workspace = "e+5" }))

-- Example special workspace (scratchpad)
hl.bind(mainMod .. " + S",         hl.dsp.workspace.toggle_special("magic"))
hl.bind(mainMod .. " + SHIFT + S", hl.dsp.window.move({ workspace = "special:magic" }))

-- Scroll through existing workspaces with mainMod + scroll
hl.bind(mainMod .. " + mouse_down", hl.dsp.focus({ workspace = "e+1" }))
hl.bind(mainMod .. " + mouse_up",   hl.dsp.focus({ workspace = "e-1" }))

-- Move/resize windows with mainMod + LMB/RMB and dragging
hl.bind(mainMod .. " + mouse:272", hl.dsp.window.drag(),   { mouse = true })
hl.bind(mainMod .. " + mouse:273", hl.dsp.window.resize(), { mouse = true })

-- Modo Estudio (Todo normal)
hl.bind(mainMod .. " + F1", hl.dsp.exec_cmd(
    'hyprctl keyword monitor "DP-2, 1920x1080@60, auto, 1" && hyprctl keyword monitor "HDMI-A-1, 1920x1080@60, auto, 1"'))

-- Modo CS2 (Principal estirado, Secundario se mantiene a la derecha)
hl.bind(mainMod .. " + F2", hl.dsp.exec_cmd(
    'hyprctl keyword monitor "DP-2, 1440x1080@60, auto, 1" && hyprctl keyword monitor "HDMI-A-1, 1920x1080@60, auto, 1"'))

-- Laptop multimedia keys for volume and LCD brightness
hl.bind("XF86AudioRaiseVolume", hl.dsp.exec_cmd("wpctl set-volume -l 1 @DEFAULT_AUDIO_SINK@ 5%+"),   { locked = true, repeating = true })
hl.bind("XF86AudioLowerVolume", hl.dsp.exec_cmd("wpctl set-volume @DEFAULT_AUDIO_SINK@ 5%-"),        { locked = true, repeating = true })
hl.bind("XF86AudioMute",        hl.dsp.exec_cmd("wpctl set-mute @DEFAULT_AUDIO_SINK@ toggle"),       { locked = true, repeating = true })
hl.bind("XF86AudioMicMute",     hl.dsp.exec_cmd("wpctl set-mute @DEFAULT_AUDIO_SOURCE@ toggle"),     { locked = true, repeating = true })
hl.bind("XF86MonBrightnessUp",  hl.dsp.exec_cmd("brightnessctl -e4 -n2 set 5%+"),                    { locked = true, repeating = true })
hl.bind("XF86MonBrightnessDown",hl.dsp.exec_cmd("brightnessctl -e4 -n2 set 5%-"),                    { locked = true, repeating = true })

-- Requires playerctl
hl.bind("XF86AudioNext",  hl.dsp.exec_cmd("playerctl next"),       { locked = true })
hl.bind("XF86AudioPause", hl.dsp.exec_cmd("playerctl play-pause"), { locked = true })
hl.bind("XF86AudioPlay",  hl.dsp.exec_cmd("playerctl play-pause"), { locked = true })
hl.bind("XF86AudioPrev",  hl.dsp.exec_cmd("playerctl previous"),   { locked = true })


--------------------------------
---- WINDOWS AND WORKSPACES ----
--------------------------------

-- See https://wiki.hypr.land/Configuring/Basics/Window-Rules/
-- See https://wiki.hypr.land/Configuring/Basics/Workspace-Rules/

-- Brave browser: override opacity
hl.window_rule({
    name  = "brave-opacity",
    match = { class = "brave-browser" },
    opacity = { active = 0.9, inactive = 0.9 },
})

-- YT Music: override opacity
hl.window_rule({
    name  = "yt-music-opacity",
    match = { class = "com.github.th_ch.youtube_music" },
    opacity = { active = 0.9, inactive = 0.9 },
})

-- Float file dialogs
hl.window_rule({
    name  = "float-dialogs",
    match = { title = "Confirmacion|Abrir Achivo" },
    float = true,
})

-- MPV fullscreen
hl.window_rule({
    name  = "mpv-fullscreen",
    match = { class = "mpv" },
    fullscreen = true,
})

-- CS2 immediate fullscreen
hl.window_rule({
    name  = "cs2-fullscreen",
    match = { class = "cs2" },
    fullscreen = true,
})

-- Discord: allow input and focus
hl.window_rule({
    name  = "discord-focus",
    match = { class = "discord" },
    allows_input       = true,
    focus_on_activate  = true,
})

-- Pavucontrol float
hl.window_rule({
    name  = "pavucontrol-float",
    match = { class = "org.pulseaudio.pavucontrol" },
    float = true,
})

-- Ueberzugpp float
hl.window_rule({
    name  = "ueberzugpp-float",
    match = { class = "^(ueberzugpp_)(.*)$" },
    float = true,
})

-- Kitty float
hl.window_rule({
    name  = "kitty-float",
    match = { class = "kitty" },
    float = true,
})

-- YT Music opacity via class match (legacy duplicate rule kept for compatibility)
hl.window_rule({
    name  = "yt-music-class-opacity",
    match = { class = "com.github.th_ch.youtube_music" },
    opacity = { active = 0.9, inactive = 0.9 },
})

-- Updating system window
hl.window_rule({
    name  = "updating-system",
    match = { title = "UPDATING_SYSTEM" },
    float  = true,
    center = true,
    size   = { width = 800, height = 500 },
})


-----------------------
----- XWAYLAND --------
-----------------------

hl.config({
    xwayland = {
        force_zero_scaling = true,
    },
})


-----------------------------
----- PLUGINS (phase 3) -----
-----------------------------

-- These option tables preserve the legacy plugin settings and require the
-- corresponding plugins to be installed and enabled by the installed Hyprland
-- plugin manager. Keep them compatible with the installed plugin versions.
-- `plugin.path` belonged to the legacy `plugin {}` loader and is intentionally
-- omitted: `hyprpm reload -n` owns plugin discovery in this configuration.
-- Compatibility risk: split-monitor-workspaces and hyprexpo are legacy binary
-- plugins, not guaranteed Lua packages. Their option names are preserved here,
-- but should be checked against the installed plugin releases before upgrading.
hl.config({
    plugin = {
        split_monitor_workspaces = {
            count = 5,
            keep_focused = 1,
            enable_notifications = 1,
            enable_persistent_workspaces = 1,
        },
        csgo_vulkan_fix = {
            fix_mouse = true,
        },
        hyprexpo = {
            columns = 3,
            gap_size = 5,
            bg_col = "rgb(111111)",
            workspace_method = "center current",
        },
        hyprbars = {
            enabled = true,
            bar_height = 20,
            bar_color = "rgb(26,27,38)",
            bar_blur = true,
            bar_title_enabled = true,
            bar_part_of_window = true,
        },
    },
})

-- Plugin helpers are deferred until after hyprpm has had a chance to load the
-- plugins during hyprland.start. Missing or incompatible helpers are ignored so
-- an optional plugin cannot prevent Hyprland from starting.
local function call_plugin_helper(plugin_name, helper_name, options)
    pcall(function()
        local plugin = hl.plugin[plugin_name]
        local helper = plugin and plugin[helper_name]
        if type(helper) == "function" then
            helper(options)
        end
    end)
end

local function register_plugin_helpers()
    -- vkfix-app is repeated in the legacy config, so use the plugin's Lua helper.
    call_plugin_helper("csgo_vulkan_fix", "vkfix_app", {
        app = "cs2",
        w = 1584,
        h = 1080,
    })
    call_plugin_helper("csgo_vulkan_fix", "vkfix_app", {
        app = "steam_app_2427520",
        w = 1920,
        h = 1080,
    })

    -- hyprbars-button is also repeated, so register each button with its helper.
    call_plugin_helper("hyprbars", "add_button", {
        bg_color = "rgb(ff4040)",
        fg_color = "rgb(ffffff)",
        size = 10,
        icon = "󰖭",
        action = "hyprctl dispatch killactive",
    })
    call_plugin_helper("hyprbars", "add_button", {
        bg_color = "rgb(eeee11)",
        fg_color = "rgb(ffffff)",
        size = 10,
        icon = "",
        action = "hyprctl dispatch fullscreen 1",
    })
end

hl.on("hyprland.start", function()
    -- The one-shot timer runs after the start hook's asynchronous hyprpm reload.
    hl.timer(register_plugin_helpers, { timeout = 5000, type = "oneshot" })
end)
