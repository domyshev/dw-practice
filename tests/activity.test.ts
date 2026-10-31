import assert from 'node:assert/strict';
import {test} from 'node:test';
import {practiceActivity,parseChartPeriod} from '../web/src/activity.ts';
import {actions,store,type Entry} from '../web/src/store.ts';
const entry=(at:string,total=10,initial=false):Entry=>({id:at,userId:'u',practice:'prostrations',large:0,small:0,repetitions:total,total,initial,occurredAt:at,timeZone:'UTC',offsetMinutes:0,note:'',createdAt:at,updatedAt:at,version:1});
test('week includes exactly the last seven display-zone days and excludes starting balances',()=>{
 const result=practiceActivity([entry('2026-10-25T00:00:00Z'),entry('2026-10-31T12:00:00Z',20),entry('2026-10-24T23:59:59Z',100),entry('2026-10-30T00:00:00Z',1000,true)],'week','UTC',new Date('2026-10-31T20:00:00Z'));
 assert.equal(result.buckets.length,7);assert.equal(result.total,30);assert.equal(result.counts.prostrations,30);assert.equal(result.buckets.at(-1)?.label,'Today');
});
test('calendar month handles leap days and groups daily values into readable weeks',()=>{
 const result=practiceActivity([entry('2024-02-01T00:00:00Z'),entry('2024-02-29T23:59:59Z'),entry('2024-03-01T00:00:00Z',999)],'month','UTC',new Date('2024-02-29T12:00:00Z'));
 assert.equal(result.total,20);assert.equal(result.buckets.length,5);assert.equal(result.buckets[0].key,'2024-02-01');assert.equal(result.buckets.at(-1)?.endKey,'2024-02-29');assert.equal(result.buckets.at(-1)?.total,10);
});
test('calendar quarter includes three months with independent practice totals',()=>{
 const result=practiceActivity([entry('2026-09-30T23:59:59Z',99),entry('2026-10-01T00:00:00Z'),{...entry('2026-11-15T00:00:00Z',20),practice:'mandala'}],'quarter','UTC',new Date('2026-10-31T12:00:00Z'));
 assert.equal(result.buckets.length,3);assert.equal(result.buckets[0].label,'Oct');assert.equal(result.buckets[2].label,'Dec');assert.equal(result.total,30);assert.equal(result.counts.mandala,20);
});
test('year and previous year respect the device time zone at a year boundary',()=>{
 const now=new Date('2026-01-01T02:00:00Z');
 const year=practiceActivity([entry('2025-06-01T00:00:00Z',30),entry('2024-06-01T00:00:00Z',20)],'year','America/New_York',now);
 const previous=practiceActivity([entry('2025-06-01T00:00:00Z',30),entry('2024-06-01T00:00:00Z',20)],'last-year','America/New_York',now);
 assert.equal(year.buckets.length,12);assert.equal(year.buckets[0].key,'2025-01-01');assert.equal(year.total,30);assert.equal(previous.buckets[0].key,'2024-01-01');assert.equal(previous.total,20);
});
test('unknown saved period safely falls back to week',()=>{assert.equal(parseChartPeriod('unknown'),'week');assert.equal(parseChartPeriod(null),'week');assert.equal(parseChartPeriod('last-year'),'last-year')});
test('Redux persists period changes without persisting account data and retains preference on logout',()=>{
 const previous=Object.getOwnPropertyDescriptor(globalThis,'localStorage');const saved=new Map<string,string>();
 Object.defineProperty(globalThis,'localStorage',{configurable:true,value:{getItem:(key:string)=>saved.get(key)||null,setItem:(key:string,value:string)=>saved.set(key,value)}});
 try{store.dispatch(actions.setChartPeriod('quarter'));assert.equal(saved.get('dw-chart-period'),'quarter');store.dispatch(actions.signedOut());assert.equal(store.getState().chartPeriod,'quarter');assert.deepEqual([...saved.keys()],['dw-chart-period'])}
 finally{if(previous)Object.defineProperty(globalThis,'localStorage',previous);else Reflect.deleteProperty(globalThis,'localStorage');store.dispatch(actions.setChartPeriod('week'))}
});
