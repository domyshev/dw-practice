import assert from 'node:assert/strict';
import {test} from 'node:test';
import {weeklyPractice} from '../web/src/week.ts';
import type {Entry} from '../web/src/store.ts';
const entry:Entry={id:'e1',userId:'u1',practice:'prostrations',large:2,small:1,repetitions:4,total:247,initial:false,occurredAt:'2026-01-20T20:15:00+01:00',timeZone:'Europe/Madrid',offsetMinutes:60,note:'',createdAt:'2026-01-20T19:15:00Z',updatedAt:'2026-01-20T19:15:00Z',version:1};
test('stacked day totals preserve each practice and exclude starting balances',()=>{
 const [day]=weeklyPractice([entry,{...entry,id:'e2',practice:'diamond-mind',total:324},{...entry,id:'e3',practice:'mandala',total:1000,initial:true}],[new Date('2026-01-20T12:00:00Z')],'Europe/Madrid');
 assert.equal(day.total,571);assert.deepEqual(day.counts,{prostrations:247,'diamond-mind':324,mandala:0,'guru-yoga':0});assert.equal(Object.values(day.counts).reduce((sum,n)=>sum+n,0),day.total);
});
test('segments use the display zone when a recorded local date differs',()=>{
 const [day]=weeklyPractice([{...entry,occurredAt:'2026-01-21T00:15:00+01:00'}],[new Date('2026-01-20T12:00:00Z')],'America/New_York');assert.equal(day.total,247);
});
test('empty days and different dates do not create phantom segments',()=>{
 const days=weeklyPractice([entry],[new Date('2026-01-19T12:00:00Z'),new Date('2026-01-20T12:00:00Z')],'Europe/Madrid');assert.equal(days[0].total,0);assert.ok(Object.values(days[0].counts).every(n=>n===0));assert.equal(days[1].total,247);
});
