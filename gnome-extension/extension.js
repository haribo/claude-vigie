// Claude Vigie — GNOME Shell top-bar indicator.
//
// A read-only client of a vigied server: it polls GET /api/sessions and
// surfaces how many sessions are calling for the operator — waiting on input,
// stalled on a tool, in error, or raising a call. It never writes into or
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

import {groupOrder, needsAttention, attentionReason, attentionIds, STATUS_ORDER} from './lib.js';

const STATUS_LABEL = {
    working: 'Working',
    thinking: 'Thinking',
    compacting: 'Compacting context',
    waiting: 'Waiting for input',
    stalled: 'Stalled on a tool',
    idle: 'Idle',
    error: 'API error',
    stale: 'Unknown (no watcher)',
    ended: 'Ended',
};

const VigieIndicator = GObject.registerClass(
class VigieIndicator extends PanelMenu.Button {
    _init(extension) {
        super._init(0.0, 'Claude Vigie');

        this._extension = extension;
        this._settings = extension.getSettings();
        this._session = new Soup.Session();
        this._timeoutId = 0;
        this._settingsChangedId = 0;
        this._callingIds = new Set(); // sessions currently calling for the operator, for edge-triggered notifications
        this._primed = false;         // first poll seeds the set without notifying (no launch storm)

        const box = new St.BoxLayout({style_class: 'panel-status-menu-box'});
        this._icon = new St.Icon({
            gicon: Gio.icon_new_for_string(`${extension.path}/icons/vigie-symbolic.svg`),
            style_class: 'system-status-icon',
        });
        this._badge = new St.Label({
            style_class: 'vigie-badge',
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
        // Everything that calls for the operator, not just `waiting`: a stalled
        // turn and a session's own call are the other two, and they are why this
        // indicator exists (#466).
        const calling = sessions.filter(needsAttention).length;

        this._notifyNewlyCalling(sessions);

        if (calling > 0) {
            this._icon.add_style_class_name('vigie-attention');
            this._badge.text = String(calling);
            this._badge.visible = true;
        } else {
            this._icon.remove_style_class_name('vigie-attention');
            this._badge.visible = false;
        }
        this._icon.remove_style_class_name('vigie-error');
        this._rebuildMenu(sessions);
    }

    // _notifyNewlyCalling fires a notification for each session that entered the
    // attention set since the last poll — waiting, stalled, error, or a call it
    // raised itself (edge-triggered, one per transition). The first poll only
    // seeds the set, so launching the extension never notifies for sessions that
    // were already calling. Observe-only: it reads and reports.
    _notifyNewlyCalling(sessions) {
        const now = attentionIds(sessions);
        if (this._primed && this._settings.get_boolean('notify')) {
            for (const s of sessions) {
                if (needsAttention(s) && !this._callingIds.has(s.id))
                    this._notifyCalling(s);
            }
        }
        this._callingIds = now;
        this._primed = true;
    }

    // The body says *why*: a stalled turn, an API error and a raised call all want
    // different things from the operator, and a notification that says only
    // "waiting for input" for all three is misleading.
    _notifyCalling(s) {
        const name = s.name || s.id;
        const context = [s.machine, s.git_branch].filter(Boolean).join(' · ');
        const who = context ? `${name} (${context})` : name;
        Main.notify('Claude Vigie', `${who} ${attentionReason(s)}`);
    }

    _showError(message) {
        this._icon.remove_style_class_name('vigie-attention');
        this._icon.add_style_class_name('vigie-error');
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
            for (const status of groupOrder(sessions, STATUS_ORDER)) {
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
        const name = s.name || s.id;
        const context = [s.machine, s.git_branch].filter(Boolean).join(' · ');
        const item = new PopupMenu.PopupMenuItem(name, {reactive: false});
        if (context) {
            const label = new St.Label({text: context, style_class: 'vigie-context'});
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

export default class VigieExtension extends Extension {
    enable() {
        this._indicator = new VigieIndicator(this);
        Main.panel.addToStatusArea(this.uuid, this._indicator);
    }

    disable() {
        this._indicator?.destroy();
        this._indicator = null;
    }
}
