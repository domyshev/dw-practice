import {isNative,shareExport} from './native.ts';
import type {Entry,Audit,User} from './store.ts';
export type ExportFormat='tsv'|'csv'|'xlsx';
export type Cell=string|number|boolean;
export type ExportTable={name:string;columns:string[];rows:Cell[][]};
const fields=['practice','large','small','repetitions','total','initial','occurredAt','timeZone','offsetMinutes','note','version','createdAt','updatedAt'] as const;
const snapshot=(entry:Entry|null):Cell[]=>fields.map(field=>entry?.[field]??'');
export function journalTables(entries:Entry[],audit:Audit[],user:User):ExportTable[]{
 return [
  {name:'Practice',columns:['entry_id','user_id','user_email',...fields],rows:entries.map(e=>[e.id,e.userId,user.email,...snapshot(e)])},
  {name:'Practice history',columns:['event_id','entry_id','operation','changed_at_utc','user_email','user_id',...fields.map(f=>`before_${f}`),...fields.map(f=>`after_${f}`)],rows:audit.map(a=>[a.id,a.entryId,a.action,a.at,user.email,(a.after||a.before)?.userId||user.id,...snapshot(a.before),...snapshot(a.after)])}
 ];
}
function safeCell(value:Cell):string{
 const text=String(value);
 // Spreadsheet applications must interpret user text as text, not formulas.
 return typeof value==='string'&&/^\s*[=+@-]/.test(text)?`'${text}`:text;
}
export function delimitedTable(table:ExportTable,delimiter:'\t'|','):string{
 const quote=(value:Cell)=>{const text=safeCell(value);return /["\r\n]/.test(text)||text.includes(delimiter)?`"${text.replaceAll('"','""')}"`:text};
 return '\uFEFF'+[table.columns,...table.rows].map(row=>row.map(quote).join(delimiter)).join('\r\n')+'\r\n';
}
export async function workbookBytes(tables:ExportTable[]):Promise<Uint8Array>{
 const module=await import('exceljs');
 const ExcelJS=module.default||module;
 const workbook=new ExcelJS.Workbook();workbook.creator='dw';workbook.created=new Date();
 for(const table of tables){
  const sheet=workbook.addWorksheet(table.name,{views:[{state:'frozen',ySplit:1}]});
  sheet.columns=table.columns.map(key=>({header:key,key,width:key.includes('note')?44:key.includes('At')||key.includes('utc')?28:20}));
  sheet.addRows(table.rows);sheet.getRow(1).font={bold:true};
  sheet.autoFilter={from:{row:1,column:1},to:{row:Math.max(1,table.rows.length+1),column:table.columns.length}};
 }
 const buffer=await workbook.xlsx.writeBuffer();return new Uint8Array(buffer);
}
export async function downloadJournal(format:ExportFormat,entries:Entry[],audit:Audit[],user:User):Promise<void>{
 const tables=journalTables(entries,audit,user);
 const payload=format==='xlsx'?await workbookBytes(tables):delimitedTable(tables[1],format==='tsv'?'\t':',');
 const filename=`dw-practice-history-${new Date().toISOString().slice(0,10)}.${format}`;
 if(isNative()){await shareExport(filename,payload);return}
 const type=format==='xlsx'?'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet':format==='tsv'?'text/tab-separated-values;charset=utf-8':'text/csv;charset=utf-8';
 const blob=new Blob([payload as BlobPart],{type});const url=URL.createObjectURL(blob);
 const link=document.createElement('a');link.href=url;link.download=filename;
 document.body.append(link);link.click();link.remove();setTimeout(()=>URL.revokeObjectURL(url),1000);
}
