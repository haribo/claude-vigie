// Claude Vigie — GNOME Shell top-bar indicator.
//
// A read-only client of a vigied server: it polls GET /api/sessions and
// surfaces how many sessions are waiting for input. It never writes into or
// drives a session (observe-only, see docs/adr/0005-observe-only.md).

import GObject from 'gi://GObject';
import St from 'gi://St';
import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import Clutter from 'gi://Clutter';
import Soup from 'gi://Soup?version=3.0';

import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import * as PanelMenu from 'resource:///org/gnome/shell/ui/panelMenu.js';
import * as PopupMenu from 'resource:///org/gnome/shell/ui/popupMenu.js';

// Statuses, most-active first, with a display label.
const STATUS_ORDER = ['working', 'waiting', 'idle', 'ended'];
const STATUS_LABEL = {
    working: 'Working',
    waiting: 'Waiting for input',
    idle: 'Idle',
    ended: 'Ended',
};

function basename(path) {
    if (!path)
        return '';
    const parts = path.replace(/\/+$/, '').split('/');
    return parts[parts.length - 1] || path;
}

const FleetIndicator = GObject.registerClass(
class FleetIndicator extends PanelMenu.Button {
    _init(extension) {
        super._init(0.0, 'Claude Vigie');

        this._extension = extension;
        this._settings = extension.getSettings();
        this._session = new Soup.Session();
        this._timeoutId = 0;
        this._settingsChangedId = 0;

        const box = new St.BoxLayout({style_class: 'panel-status-menu-box'});
        this._icon = new St.Icon({
            gicon: Gio.icon_new_for_string(`${extension.path}/icons/vigie-symbolic.svg`),
            style_class: 'system-status-icon',
        });
        this._badge = new St.Label({
            style_class: 'cf-badge',
            y_align: Clutter.ActorAlign.CENTER,
            visible: false,
        });
        box.add_child(this._icon);
        box.add_child(this._badge);
        this.add_child(box);

        this._settingsChangedId = this._settings.connect('changed', () => this._restart());
        this._restart();
    }

    _restart() {
        if (this._timeoutId) {
            GLib.source_remove(this._timeoutId);
            this._timeoutId = 0;
        }
        const interval = Math.max(2, this._settings.get_int('poll-interval'));
        this._refresh();
        this._timeoutId = GLib.timeout_add_seconds(GLib.PRIORITY_DEFAULT, interval, () => {
            this._refresh();
            return GLib.SOURCE_CONTINUE;
        });
    }

    _refresh() {
        const base = this._settings.get_string('server-url').replace(/\/+$/, '');
        if (!base) {
            this._showError('Set the server URL in preferences');
            return;
        }
        const token = this._settings.get_string('token');
        const msg = Soup.Message.new('GET', `${base}/api/sessions`);
        if (token)
            msg.request_headers.append('Authorization', `Bearer ${token}`);

        this._session.send_and_read_async(msg, GLib.PRIORITY_DEFAULT, null, (session, res) => {
            let bytes;
            try {
                bytes = session.send_and_read_finish(res);
            } catch (e) {
                this._showError('Server unreachable');
                console.debug(`vigie: ${e}`);
                return;
            }
            const status = msg.get_status();
            if (status !== Soup.Status.OK) {
                this._showError(status === Soup.Status.UNAUTHORIZED ? 'Invalid token' : `HTTP ${status}`);
                return;
            }
            try {
                const text = new TextDecoder().decode(bytes.get_data());
                this._update(JSON.parse(text));
            } catch (e) {
                this._showError('Bad response');
                console.debug(`vigie: ${e}`);
            }
        });
    }

    _update(sessions) {
        if (!Array.isArray(sessions))
            sessions = [];
        const waiting = sessions.filter(s => s.status === 'waiting').length;

        if (waiting > 0) {
            this._icon.add_style_class_name('cf-attention');
            this._badge.text = String(waiting);
            this._badge.visible = true;
        } else {
            this._icon.remove_style_class_name('cf-attention');
            this._badge.visible = false;
        }
        this._icon.remove_style_class_name('cf-error');
        this._rebuildMenu(sessions);
    }

    _showError(message) {
        this._icon.remove_style_class_name('cf-attention');
        this._icon.add_style_class_name('cf-error');
        this._badge.visible = false;
        this.menu.removeAll();
        this.menu.addMenuItem(new PopupMenu.PopupMenuItem(`⚠ ${message}`, {reactive: false}));
        this._addPreferencesItem();
    }

    _rebuildMenu(sessions) {
        this.menu.removeAll();

        if (sessions.length === 0) {
            this.menu.addMenuItem(new PopupMenu.PopupMenuItem('No sessions', {reactive: false}));
        } else {
            for (const status of STATUS_ORDER) {
                const group = sessions.filter(s => s.status === status);
                if (group.length === 0)
                    continue;
                this.menu.addMenuItem(new PopupMenu.PopupSeparatorMenuItem(
                    `${STATUS_LABEL[status] ?? status} (${group.length})`));
                for (const s of group)
                    this.menu.addMenuItem(this._sessionItem(s));
            }
        }

        this.menu.addMenuItem(new PopupMenu.PopupSeparatorMenuItem());
        this._addPreferencesItem();
    }

    _sessionItem(s) {
        const name = s.title || basename(s.project_dir) || s.id;
        const context = [s.machine, s.git_branch].filter(Boolean).join(' · ');
        const item = new PopupMenu.PopupMenuItem(name, {reactive: false});
        if (context) {
            const label = new St.Label({text: context, style_class: 'cf-context'});
            item.add_child(label);
        }
        return item;
    }

    _addPreferencesItem() {
        const item = new PopupMenu.PopupMenuItem('Preferences');
        item.connect('activate', () => this._extension.openPreferences());
        this.menu.addMenuItem(item);
    }

    destroy() {
        if (this._timeoutId) {
            GLib.source_remove(this._timeoutId);
            this._timeoutId = 0;
        }
        if (this._settingsChangedId) {
            this._settings.disconnect(this._settingsChangedId);
            this._settingsChangedId = 0;
        }
        this._session?.abort();
        this._session = null;
        super.destroy();
    }
});

export default class ClaudeFleetExtension extends Extension {
    enable() {
        this._indicator = new FleetIndicator(this);
        Main.panel.addToStatusArea(this.uuid, this._indicator);
    }

    disable() {
        this._indicator?.destroy();
        this._indicator = null;
    }
}
