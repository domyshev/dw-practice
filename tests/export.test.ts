import assert from 'node:assert/strict';
import {test} from 'node:test';
import {journalTables,delimitedTable,workbookBytes} from '../web/src/export.ts';
import type {Entry,Audit} from '../web/src/store.ts';
const user={id:'u1',email:'demo@example.test'};
const entry:Entry={id:'e1',userId:'u1',practice:'mandala',large:2,small:1,repetitions:4,total:247,initial:true,occurredAt:'2026-01-20T20:15:00+05:45',timeZone:'Asia/Kathmandu',offsetMinutes:345,note:'A note, with "quotes"\nand a\ttab',createdAt:'2026-01-20T14:30:00Z',updatedAt:'2026-01-20T14:30:00Z',version:1};
const updated={...entry,repetitions:5,total:248,note:'=not a formula',version:2};
const events:Audit[]=[{id:'a3',entryId:'e1',action:'deleted',at:'2026-01-22T10:00:00Z',before:updated,after:null},{id:'a2',entryId:'e1',action:'updated',at:'2026-01-21T10:00:00Z',before:entry,after:updated},{id:'a1',entryId:'e1',action:'created',at:'2026-01-20T14:30:00Z',before:null,after:entry}];

test('history exports retain deleted entries, complete before/after snapshots and original zones',()=>{
 const tables=journalTables([],events,user);assert.equal(tables[0].rows.length,0);const table=tables[1];assert.equal(table.rows.length,3);
 const cell=(row:number,key:string)=>table.rows[row][table.columns.indexOf(key)];
 assert.equal(cell(0,'operation'),'deleted');assert.equal(cell(0,'before_total'),248);assert.equal(cell(0,'after_total'),'');
 assert.equal(cell(1,'before_total'),247);assert.equal(cell(1,'after_total'),248);assert.equal(cell(1,'before_note'),entry.note);assert.equal(cell(1,'after_note'),'=not a formula');assert.equal(cell(1,'before_offsetMinutes'),345);assert.equal(cell(1,'before_timeZone'),'Asia/Kathmandu');assert.equal(cell(2,'after_initial'),true);
});

test('CSV safely quotes commas, quotes and line breaks and emits UTF-8 BOM',()=>{
 const csv=delimitedTable({name:'sample',columns:['note','number'],rows:[[entry.note,247]]},',');
 assert.ok(csv.startsWith('\uFEFFnote,number\r\n'));assert.ok(csv.includes('"A note, with ""quotes""\nand a\ttab",247\r\n'));
});

test('TSV quotes tabs and neutralizes formula-like user strings while retaining numeric cells',()=>{
 const tsv=delimitedTable({name:'sample',columns:['text','number'],rows:[['=HYPERLINK("x")',-60],[' +SUM(A1)',12],['a\tb',2],['@reference',3]]},'\t');
 assert.ok(tsv.includes('"\'=HYPERLINK(""x"")"\t-60'));assert.ok(tsv.includes("' +SUM(A1)\t12"));assert.ok(tsv.includes('"a\tb"\t2'));assert.ok(tsv.includes("'@reference\t3"));
});

test('XLSX opens with two sheets, typed counts, full Unicode notes and audit snapshots',async()=>{
 const bytes=await workbookBytes(journalTables([updated],events,user));assert.equal(bytes[0],0x50);assert.equal(bytes[1],0x4b);
 const module=await import('exceljs');const ExcelJS=module.default||module;const workbook=new ExcelJS.Workbook();await workbook.xlsx.load(bytes);
 assert.deepEqual(workbook.worksheets.map(s=>s.name),['Practice','Practice history']);
 const practice=workbook.getWorksheet('Practice')!,history=workbook.getWorksheet('Practice history')!;
 const read=(sheet:typeof practice,row:number,column:string)=>{const index=sheet.getRow(1).values.indexOf(column);assert.ok(index>0);return sheet.getRow(row).getCell(index).value};
 assert.equal(read(practice,2,'total'),248);assert.equal(read(practice,2,'offsetMinutes'),345);assert.equal(read(practice,2,'note'),'=not a formula');
 assert.equal(read(history,3,'before_note'),entry.note);assert.equal(read(history,3,'after_total'),248);assert.equal(history.rowCount,4);
});

test('empty journals export valid spreadsheet headers',async()=>{
 const tables=journalTables([],[],user);assert.ok(delimitedTable(tables[1],',').includes('operation'));
 const bytes=await workbookBytes(tables);assert.ok(bytes.length>1000);
});
