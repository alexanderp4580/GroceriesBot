"""One-time migration helper. Python is not a bot runtime dependency."""
import argparse
import hashlib
import json
import math
import os
import sqlite3
import zlib
from datetime import datetime, timedelta, timezone
from pathlib import Path
from zoneinfo import ZoneInfo

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_SETTINGS = {
    'llm.model': 'anthropic/claude-sonnet-4.5',
    'llm.max_tool_rounds': 6,
    'llm.max_output_tokens': 800,
    'llm.timeout_seconds': 30,
    'llm.context_max_chars': 12000,
    'memory.max_messages_per_chat': 20,
    'memory.max_age_days': 30,
    'telegram.poll_timeout_seconds': 50,
    'telegram.allowed_user_ids': [],
    'photos.max_bytes': 4194304,
    'updates.retention_days': 30,
    'confirm.expiry_seconds': 300,
    'display.time_zone': 'Europe/Amsterdam',
    'list.category_order': ['Quick / ready food','Vegetables','Fruits','Vegan','Meat','Fish','Bread','Pantry','Candy','Bakery','Dairy','Household','Pharmacy','Drinks'],
    'list.max_context_items': 200,
    'history.max_suggestions': 20,
    'processing.timeout_seconds': 120,
}


def dumps(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':'))


def value(cell):
    effective = cell.get('effectiveValue', {})
    for key in ('numberValue','stringValue','boolValue'):
        if key in effective:
            return effective[key]
    return cell.get('formattedValue', '')


def timestamp(cell, zone, optional=False):
    v = value(cell)
    if v == '' and optional:
        return None
    if isinstance(v, (int,float)) and not isinstance(v, bool) and math.isfinite(v):
        dt = datetime(1899,12,30) + timedelta(seconds=round(v*86400))
    elif isinstance(v, str):
        dt = datetime.fromisoformat(v.replace('Z','+00:00'))
    else:
        raise ValueError('Invalid date')
    if dt.tzinfo is None:
        z = ZoneInfo(zone)
        candidates = {dt.replace(tzinfo=z,fold=fold).astimezone(timezone.utc)
            for fold in (0,1)
            if dt.replace(tzinfo=z,fold=fold).astimezone(timezone.utc).astimezone(z).replace(tzinfo=None) == dt}
        if len(candidates) != 1:
            raise ValueError('Ambiguous or nonexistent local date')
        dt = candidates.pop()
    return dt.astimezone(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')


def summary(db, already=False):
    return {
        'spreadsheets': db.execute('SELECT count(DISTINCT spreadsheet_id) FROM source_sheets').fetchone()[0],
        'sheets': db.execute('SELECT count(*) FROM source_sheets').fetchone()[0],
        'source_rows': db.execute('SELECT count(*) FROM source_rows').fetchone()[0],
        'groceries': db.execute('SELECT count(*) FROM groceries').fetchone()[0],
        'pending': db.execute('SELECT count(*) FROM groceries WHERE purchased_at IS NULL AND deleted_at IS NULL').fetchone()[0],
        'purchased': db.execute('SELECT count(*) FROM groceries WHERE purchased_at IS NOT NULL AND deleted_at IS NULL').fetchone()[0],
        'issues': db.execute('SELECT count(*) FROM import_issues').fetchone()[0],
        'already_imported': already,
    }


def import_snapshot(books, database_path, chat_id):
    os.umask(0o077)
    path = Path(database_path)
    path.parent.mkdir(parents=True,exist_ok=True)
    payload = dumps(books).encode()
    digest = hashlib.sha256(payload).hexdigest()
    now = datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
    db = sqlite3.connect(path, timeout=5)
    try:
        os.chmod(path,0o600)
        if db.execute('PRAGMA user_version').fetchone()[0] > 1:
            raise ValueError('Database schema is newer than this importer')
        db.execute('PRAGMA foreign_keys=ON')
        db.execute('PRAGMA journal_mode=WAL')
        db.execute('PRAGMA synchronous=FULL')
        db.execute('PRAGMA busy_timeout=5000')
        db.execute('PRAGMA cache_size=-2048')
        db.executescript((ROOT/'migrations/001_initial.sql').read_text())
        if db.execute('SELECT id FROM source_snapshots WHERE digest=?',(digest,)).fetchone():
            return summary(db,already=True)
        if db.execute('SELECT 1 FROM source_snapshots UNION ALL SELECT 1 FROM groceries LIMIT 1').fetchone():
            raise ValueError('Database already contains a different snapshot or runtime groceries; use a new database')
        with db:
            sid = db.execute('INSERT INTO source_snapshots(digest,fetched_at,payload_zlib) VALUES(?,?,?)',
                (digest,now,zlib.compress(payload))).lastrowid
            for book in books:
                props = book['properties']
                for sheet in book.get('sheets',[]):
                    sp = sheet['properties']
                    grid = sp.get('gridProperties',{})
                    sheet_id = db.execute('INSERT INTO source_sheets(snapshot_id,spreadsheet_id,sheet_id,title,locale,time_zone,grid_rows,grid_columns) VALUES(?,?,?,?,?,?,?,?)',
                        (sid,book['spreadsheetId'],sp['sheetId'],sp['title'],props.get('locale',''),props['timeZone'],grid.get('rowCount',0),grid.get('columnCount',0))).lastrowid
                    rows = {}
                    for block in sheet.get('data',[]):
                        for offset,row in enumerate(block.get('rowData',[])):
                            cells = row.get('values',[])
                            if any(value(c) != '' or c.get('userEnteredValue') or c.get('note') for c in cells):
                                number = block.get('startRow',0)+offset+1
                                target = rows.setdefault(number,{})
                                for col,c in enumerate(cells,block.get('startColumn',0)):
                                    target[col] = c
                    header = None
                    for number,columns in sorted(rows.items()):
                        cells = [columns.get(i,{}) for i in range(max(columns)+1)] if columns else []
                        compact = [{k:c[k] for k in ('userEnteredValue','effectiveValue','formattedValue','note') if k in c} for c in cells]
                        row_id = db.execute('INSERT INTO source_rows(source_sheet_id,row_number,cells_json) VALUES(?,?,?)',
                            (sheet_id,number,dumps(compact))).lastrowid
                        if sp['title'] != 'Groceries':
                            continue
                        if header is None:
                            names = [str(value(c)).strip() for c in cells]
                            required = {'ID','Name','Category','Quantity','Added DateTime','Purchase DateTime'}
                            if not required.issubset(names):
                                raise ValueError('Groceries header is missing required columns')
                            header = {name:names.index(name) for name in required}
                            continue
                        fields = {key: cells[index] if index<len(cells) else {} for key,index in header.items()}
                        name = str(value(fields['Name'])).strip()
                        issue = None
                        if not name:
                            issue = 'missing_name'
                        try:
                            q = value(fields['Quantity'])
                            if isinstance(q,bool):
                                raise ValueError()
                            quantity = float(q)
                            if not math.isfinite(quantity) or quantity<=0:
                                raise ValueError()
                        except (ValueError,TypeError):
                            issue = issue or 'invalid_quantity'
                        try:
                            added = timestamp(fields['Added DateTime'],props['timeZone'])
                        except (ValueError,OverflowError):
                            issue = issue or 'invalid_added_at'
                        try:
                            purchased = timestamp(fields['Purchase DateTime'],props['timeZone'],optional=True)
                        except (ValueError,OverflowError):
                            issue = issue or 'invalid_purchased_at'
                        if issue:
                            db.execute('INSERT INTO import_issues(source_row_id,code) VALUES(?,?)',(row_id,issue))
                            continue
                        legacy = value(fields['ID'])
                        if isinstance(legacy,float) and legacy.is_integer():
                            legacy = int(legacy)
                        db.execute('INSERT INTO groceries(chat_id,legacy_id,name,category,quantity,added_at,purchased_at,source_row_id) VALUES(?,?,?,?,?,?,?,?)',
                            (chat_id,str(legacy),name,str(value(fields['Category'])).strip(),quantity,added,purchased,row_id))
            for key,v in {**DEFAULT_SETTINGS,'telegram.allowed_chat_ids':[chat_id]}.items():
                db.execute('INSERT OR IGNORE INTO settings(key,value_json,updated_at) VALUES(?,?,?)',(key,dumps(v),now))
            db.execute('INSERT OR IGNORE INTO chat_state(chat_id,updated_at) VALUES(?,?)',(chat_id,now))
        if db.execute('PRAGMA integrity_check').fetchone()[0] != 'ok':
            raise RuntimeError('SQLite integrity check failed')
        return summary(db)
    finally:
        db.close()


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Import a private Sheets JSON snapshot into SQLite')
    parser.add_argument('snapshot',type=Path)
    parser.add_argument('--database',type=Path,default=ROOT/'data/groceriesbot.sqlite3')
    parser.add_argument('--chat-id',type=int,required=True)
    args = parser.parse_args()
    print(json.dumps(import_snapshot(json.loads(args.snapshot.read_text()),args.database,args.chat_id),indent=2))
