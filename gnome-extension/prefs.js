// Preferences for the Claude Vigie indicator: server URL, token, poll interval.

import Adw from 'gi://Adw';
import Gio from 'gi://Gio';
import Gtk from 'gi://Gtk';

import {ExtensionPreferences} from 'resource:///org/gnome/Shell/Extensions/js/extensions/prefs.js';

export default class VigiePreferences extends ExtensionPreferences {
    fillPreferencesWindow(window) {
        const settings = this.getSettings();

        const page = new Adw.PreferencesPage();
        const group = new Adw.PreferencesGroup({
            title: 'Fleet server',
            description: 'The vigied server this indicator reads from (read-only).',
        });
        page.add(group);

        const urlRow = new Adw.EntryRow({title: 'Server URL'});
        settings.bind('server-url', urlRow, 'text', Gio.SettingsBindFlags.DEFAULT);
        group.add(urlRow);

        const tokenRow = new Adw.PasswordEntryRow({title: 'Token'});
        settings.bind('token', tokenRow, 'text', Gio.SettingsBindFlags.DEFAULT);
        group.add(tokenRow);

        // poll-interval is an integer key; bind manually to avoid int/double
        // mismatches with Adw.SpinRow's double 'value' property.
        const pollRow = new Adw.SpinRow({
            title: 'Poll interval (seconds)',
            adjustment: new Gtk.Adjustment({lower: 2, upper: 300, step_increment: 1}),
        });
        pollRow.set_value(settings.get_int('poll-interval'));
        pollRow.connect('notify::value', () => {
            const v = pollRow.get_value();
            if (v !== settings.get_int('poll-interval'))
                settings.set_int('poll-interval', v);
        });
        settings.connect('changed::poll-interval', () => {
            pollRow.set_value(settings.get_int('poll-interval'));
        });
        group.add(pollRow);

        const notifyRow = new Adw.SwitchRow({
            title: 'Desktop notifications',
            subtitle: 'Notify when a session starts calling for you — waiting, in error, or raising a call',
        });
        settings.bind('notify', notifyRow, 'active', Gio.SettingsBindFlags.DEFAULT);
        group.add(notifyRow);

        window.add(page);
    }
}
