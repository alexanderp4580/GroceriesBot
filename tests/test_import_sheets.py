import json
import sqlite3
import tempfile
import unittest
import zlib
from pathlib import Path

try:
    from scripts.import_sheets import import_snapshot
except ImportError:
    import_snapshot = None


def cell(value):
    key = 'numberValue' if isinstance(value, (int, float)) else 'stringValue'
    return {'userEnteredValue': {key: value}, 'effectiveValue': {key: value}, 'formattedValue': str(value)}


def fixture():
    return [{'spreadsheetId': 'fixture', 'properties': {'locale': 'en_US', 'timeZone': 'Europe/Amsterdam'},
        'sheets': [{'properties': {'sheetId': 0, 'title': 'Groceries', 'gridProperties': {'rowCount': 100, 'columnCount': 6}},
            'data': [{'rowData': [
                {'values': [cell(x) for x in ['ID', 'Name', 'Category', 'Quantity', 'Added DateTime', 'Purchase DateTime']]},
                {'values': [cell(x) for x in [7, 'Oat milk', 'Dairy', 2, 46116.5, '']]},
                {'values': []},
                {'values': [cell(x) for x in [42, 'Bread', 'Bread', 1, 46116.5, 46119.5]]},
            ]}]},
            {'properties': {'sheetId': 9, 'title': 'Notes', 'hidden': True, 'gridProperties': {'rowCount': 1, 'columnCount': 1}},
            'data': [{'rowData': [{'values': [{'userEnteredValue': {'formulaValue': '=1+1'},
                'effectiveValue': {'numberValue': 2}, 'formattedValue': '2', 'note': 'keep me'}]}]}]}]}]


class ImportTest(unittest.TestCase):
    def setUp(self):
        self.assertTrue(callable(import_snapshot), 'SQLite sheet importer has not been implemented')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.db = Path(self.temp.name) / 'groceries.sqlite3'

    def test_preserves_full_snapshot_and_normalizes_dates_and_id_gaps(self):
        data = fixture()
        report = import_snapshot(data, self.db, -123)
        self.assertEqual(report['groceries'], 2)
        self.assertEqual(report['pending'], 1)
        self.assertEqual(report['purchased'], 1)
        with sqlite3.connect(self.db) as db:
            self.assertEqual(db.execute('SELECT legacy_id, quantity, added_at, purchased_at FROM groceries ORDER BY id').fetchall(),
                [('7', 2.0, '2026-04-04T10:00:00Z', None), ('42', 1.0, '2026-04-04T10:00:00Z', '2026-04-07T10:00:00Z')])
            payload = db.execute('SELECT payload_zlib FROM source_snapshots').fetchone()[0]
            self.assertEqual(json.loads(zlib.decompress(payload)), data)
            self.assertEqual(db.execute('SELECT count(*) FROM source_sheets').fetchone()[0], 2)
            self.assertEqual(db.execute('SELECT row_number FROM source_rows WHERE source_sheet_id=1 ORDER BY row_number').fetchall(), [(1,), (2,), (4,)])

    def test_repeat_import_preserves_runtime_settings_and_does_not_duplicate(self):
        import_snapshot(fixture(), self.db, -123)
        with sqlite3.connect(self.db) as db:
            db.execute("UPDATE settings SET value_json='3' WHERE key='llm.max_tool_rounds'")
        report = import_snapshot(fixture(), self.db, -123)
        self.assertTrue(report['already_imported'])
        with sqlite3.connect(self.db) as db:
            self.assertEqual(db.execute('SELECT count(*) FROM groceries').fetchone()[0], 2)
            self.assertEqual(db.execute("SELECT value_json FROM settings WHERE key='llm.max_tool_rounds'").fetchone()[0], '3')

    def test_bad_quantity_is_quarantined_without_losing_source_data(self):
        data = fixture()
        data[0]['sheets'][0]['data'][0]['rowData'][1]['values'][3] = cell('many')
        report = import_snapshot(data, self.db, -123)
        self.assertEqual(report['groceries'], 1)
        self.assertEqual(report['issues'], 1)
        with sqlite3.connect(self.db) as db:
            self.assertEqual(db.execute('SELECT code FROM import_issues').fetchone()[0], 'invalid_quantity')
            self.assertEqual(db.execute('SELECT count(*) FROM source_rows').fetchone()[0], 4)

    def test_changed_snapshot_requires_new_database(self):
        import_snapshot(fixture(), self.db, -123)
        changed = fixture()
        changed[0]['sheets'][0]['data'][0]['rowData'][1]['values'][1] = cell('Different milk')
        with self.assertRaisesRegex(ValueError, 'already contains a different snapshot'):
            import_snapshot(changed, self.db, -123)
        with sqlite3.connect(self.db) as db:
            self.assertEqual(db.execute('SELECT name FROM groceries ORDER BY id LIMIT 1').fetchone()[0], 'Oat milk')

    def test_invalid_dates_stay_in_source_and_runtime_state_uses_same_database(self):
        data = fixture()
        data[0]['sheets'][0]['data'][0]['rowData'][1]['values'][4] = cell('04/05/2026')
        report = import_snapshot(data, self.db, -123)
        self.assertEqual(report['issues'], 1)
        with sqlite3.connect(self.db) as db:
            self.assertEqual(db.execute('SELECT code FROM import_issues').fetchone()[0], 'invalid_added_at')
            db.execute("INSERT INTO conversation_messages(chat_id,user_id,role,content,created_at) VALUES(-123,8,'user','milk','2026-10-12T00:00:00Z')")
            db.execute("INSERT INTO pending_actions(chat_id,user_id,kind,payload_json,list_revision,expires_at) VALUES(-123,8,'clarify','{}',0,'2026-10-12T00:05:00Z')")
            db.execute("INSERT INTO telegram_updates(update_id,chat_id,payload_json,received_at) VALUES(20,-123,'{}','2026-10-12T00:00:00Z')")
            db.execute("INSERT INTO tool_actions(update_id,round_index,action_index,tool_name,arguments_json,result_json,created_at) VALUES(20,0,0,'list','{}','{}','2026-10-12T00:00:00Z')")
            db.execute("INSERT INTO outbox(update_id,chat_id,payload_json,created_at) VALUES(20,-123,'{}','2026-10-12T00:00:00Z')")
            self.assertEqual(db.execute('PRAGMA integrity_check').fetchone()[0], 'ok')


if __name__ == '__main__':
    unittest.main()
