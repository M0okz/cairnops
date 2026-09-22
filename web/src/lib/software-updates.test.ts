import test from 'node:test';
import assert from 'node:assert/strict';
// @ts-ignore -- Node executes this test directly with its TypeScript loader.
import { currentAnalysis, safeReleaseURL, type SoftwareService } from './software-updates.ts';
test('only the matching current comparison is presented as current', () => {
 const service: SoftwareService = {id:'service',target_id:'target',name:'Service',installed_version:'2.6.0',target_version:'2.10.0',observed_at:null,known:true,source:{kind:'github',url:'https://github.com/example/project',software:'Project'},source_origin: "manual", suggested_source:null,confirmed_at:null,revision:4,state:'pending',last_error:'',checked_at:null,collection:null,collection_revision:null,history:[],analyses:[{notes:[],id:1,revision:3,installed_version:'2.6.0',target_version:'2.9.1',source:{kind:'github',url:'https://github.com/example/project',software:'Project'},result:{overview:[],details:[]},model:'test',created_at:'2026-09-21T00:00:00Z',current:true}]};
 assert.equal(currentAnalysis(service),undefined);
 service.analyses[0].revision=4; service.analyses[0].target_version='2.10.0';assert.equal(currentAnalysis(service)?.id,1);
});
test('release links reject executable schemes and embedded credentials',()=>{
 for(const url of ['javascript:alert(1)','data:text/html,unsafe','https://user:secret@example.com'])assert.equal(safeReleaseURL(url),undefined);
 assert.equal(safeReleaseURL('https://github.com/example/project/releases'),'https://github.com/example/project/releases');
});
