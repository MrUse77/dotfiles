#
# ~/.zshrc - Versión Robusta (Host & Containers)
#
# Si no es interactivo, salir
[[ $- != *i* ]] && return

# --- VARIABLES DE ENTORNO ---
export EDITOR="nvim"
export PATH="$HOME/.local/bin:$PATH"
export YAZI_IMAGE_ADAPTER=kitty
export NIXPKGS_ALLOW_UNFREE=1

# Configuración Wayland/Qt (Solo si estamos en el host)
if [[ -z "$CONTAINER_ID" ]]; then
    export QT_QPA_PLATFORM=wayland
    export QT_QPA_PLATFORMTHEME=qt5ct
    export QT_STYLE_OVERRIDE=kvantum
fi

# --- SSH AGENT ---
# Iniciar ssh-agent automáticamente si no está corriendo
if ! pgrep -u "$USER" ssh-agent > /dev/null; then
    ssh-agent -t 1h > "$XDG_RUNTIME_DIR/ssh-agent.env"
fi
if [[ ! -f "$SSH_AUTH_SOCK" ]]; then
    source "$XDG_RUNTIME_DIR/ssh-agent.env" >/dev/null
fi

# --- DETECCIÓN DE HERRAMIENTAS ---

# Bun
if [[ -d "$HOME/.bun" ]]; then
    export BUN_INSTALL="$HOME/.bun"
    export PATH="$BUN_INSTALL/bin:$PATH"
fi

# FNM (Node Manager) - Solo cargar si el binario existe
if (( $+commands[fnm] )); then
    eval "$(fnm env)"
fi

# Zoxide (Sustituto de cd)
if (( $+commands[zoxide] )); then
    eval "$(zoxide init zsh)"
fi

# Oh My Posh (Prompt)
# Asumimos queval "$(direnv hook zsh)"e está en ~/.local/bin que ya agregamos al PATH
if (( $+commands[oh-my-posh] )); then
    eval "$(oh-my-posh init zsh --config "~/oh-my-posh/tokyonight_storm.omp.json")"
else
    # Prompt minimalista de respaldo si no hay OhMyPosh
    PS1='[%n@%m %W]\$ '
fi
eval "$(direnv hook zsh)"
# --- ALIAS INTELIGENTES ---
# Solo usamos eza/bat si están instalados, si no, volvemos a los básicos
if (( $+commands[eza] )); then
    alias ls='eza --icons=always --color=always -a'
    alias ll='eza --icons=always --color=always -la'
else    alias ls='ls --color=auto -a'
    alias ll='ls -la'
fi

if (( $+commands[bat] )); then
    alias cat="bat --theme=base16"
else
    alias cat="cat"
fi

alias grep='grep --color=auto'
alias icat='kitten icat'
alias s='kitten ssh'

# --- PLUGINS (Carga Segura) ---
# --- COMPLETION ---eval "$(direnv hook zsh)"
#
source "$HOME/.zsh_plugins/fzf-tab/fzf-tab.zsh"
source "$HOME/.zsh_plugins/zsh-autosuggestions/zsh-autosuggestions.zsh"
source "$HOME/.zsh_plugins/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh"
source "$HOME/.zsh_plugins/zsh-history-substring-search/zsh-history-substring-search.zsh"

autoload -Uz compinit

local zcompdump="$HOME/.config/zsh/zcompdump"

if [[ -n "$zcompdump"(#qN.mh+24) ]]; then
    compinit -i -d "$zcompdump"
else
    compinit -C -d "$zcompdump"
fi

if [[ ! -f "${zcompdump}.zwc" || "$zcompdump" -nt "${zcompdump}.zwc" ]]; then
    zcompile -U "$zcompdump"
fi


autoload -Uz add-zsh-hook
autoload -Uz vcs_info
precmd () { vcs_info }
_comp_options+=(globdots)

zstyle ':completion:*' menu select
zstyle ':completion:*:descriptions' format '[%d]'
zstyle ':completion:*' list-colors ${(s.:.)LS_COLORS}
zstyle ':completion:*' matcher-list \
		'm:{a-zA-Z}={A-Za-z}' \
		'+r:|[._-]=* r:|=*' \
		'+l:|=*'
zstyle ':vcs_info:*' formats ' %B%s-[%F{magenta}%f %F{yellow}%b%f]-'
zstyle ':fzf-tab:*' fzf-flags --style=full --height=90% --pointer '>' \
                --color 'pointer:green:bold,bg+:-1:,fg+:green:bold,info:blue:bold,marker:yellow:bold,hl:gray:bold,hl+:yellow:bold' \
                --input-label ' Search ' --color 'input-border:blue,input-label:blue:bold' \
                --list-label ' Results ' --color 'list-border:green,list-label:green:bold' \
                --preview-label ' Preview ' --color 'preview-border:magenta,preview-label:magenta:bold'
zstyle ':fzf-tab:complete:cd:*' fzf-preview 'eza -1 --icons=always --color=always -a $realpath'
zstyle ':fzf-tab:complete:eza:*' fzf-preview 'eza -1 --icons=always --color=always -a $realpath'
zstyle ':fzf-tab:complete:bat:*' fzf-preview 'bat --color=always --theme=base16 $realpath'
zstyle ':fzf-tab:*' fzf-bindings 'space:accept'
zstyle ':fzf-tab:*' accept-line enter



# --- BINDKEYS ---
bindkey '^[[A' history-substring-search-up
bindkey '^[[B' history-substring-search-down
bindkey '^[[3~' delete-char
bindkey "^[[H" beginning-of-line
bindkey "^[[F" end-of-line

# --- FUNCIONES DE TÍTULO ---
function xterm_title_precmd () {
    print -Pn -- '\e]2;%n@%m %~\a'
}
function xterm_title_preexec () {
    print -Pn -- '\e]2;%n@%m %~ %# ' && print -n -- "${(q)1}\a"
}

if [[ "$TERM" == (kitty*|alacritty*|tmux*|screen*|xterm*) ]]; then
    autoload -Uz add-zsh-hook
    add-zsh-hook -Uz precmd xterm_title_precmd
    add-zsh-hook -Uz preexec xterm_title_preexec
fi

# --- MANTENIMIENTO (Solo Host) ---
if [[ -z "$CONTAINER_ID" ]]; then
    alias mirrors="sudo reflector --latest 5 --country 'United States' --age 6 --sort rate --save /etc/pacman.d/mirrorlist"
    alias update="paru -Syu --nocombinedupgrade"
    alias grub-update="sudo grub-mkconfig -o /boot/grub/grub.cfg"
fi

# --- HISTORIAL ---
HISTFILE=~/.config/zsh/zhistory
HISTSIZE=5000
SAVEHIST=5000
setopt appendhistory sharehistory hist_ignore_space hist_ignore_all_dups

bindkey -M viins '^?' backward-delete-char
bindkey -M viins '^h' backward-delete-char

# Added by LM Studio CLI (lms)
export PATH="$PATH:$HOME/.lmstudio/bin"
# End of LM Studio CLI section

