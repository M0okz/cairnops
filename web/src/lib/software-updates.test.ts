import test from 'node:test';
import assert from 'node:assert/strict';
// @ts-ignore -- Node executes this test directly with its TypeScript loader.
import { compareServices, currentAnalysis, currentCollection, softwareJournal, rateLimitedHost, safeReleaseURL, type SoftwareService } from './software-updates.ts';
function service(overrides: Partial<SoftwareService>): SoftwareService {
 return {id:'service',target_id:'target',resource_name:'Service',name:'owner/service',installed_version:'2.6.0',target_version:'2.10.0',observed_at:null,known:true,situation:'update',level:'minor',group:'apply',approved:false,skipped:false,security_mentioned:false,source:{kind:'github',url:'https://github.com/example/project',software:'Project'},source_origin:'manual',suggested_source:null,confirmed_at:null,revision:4,state:'pending',last_error:'',checked_at:null,next_check_at:null,collection:null,collection_revision:null,history:[],events:[],analyses:[],archives:[],...overrides};
}
test('only the matching current comparison is presented as current', () => {
 const current = service({analyses:[{notes:[],id:1,revision:3,installed_version:'2.6.0',target_version:'2.9.1',source:{kind:'github',url:'https://github.com/example/project',software:'Project'},result:{overview:[],details:[]},model:'test',created_at:'2026-09-21T00:00:00Z',current:true}]});
 assert.equal(currentAnalysis(current),undefined);
 current.analyses[0].revision=4; current.analyses[0].target_version='2.10.0';assert.equal(currentAnalysis(current)?.id,1);
});
test('notes from before an installed version change are treated as past notes', () => {
 const collection = {installed_version:'2.6.0',target_version:'2.10.0',notes:[],incomplete:false};
 const updated = service({installed_version:'2.10.0',target_version:'2.10.0',revision:5,collection_revision:4,collection});
 assert.equal(currentCollection(updated),null);
 updated.installed_version='2.6.0';updated.revision=4;
 assert.equal(currentCollection(updated),collection);
});
test('observations, official notes and summaries form one newest-first journal', () => {
 const base = service({
  installed_version:'2.10.0',target_version:'2.10.0',revision:5,
  events:[{kind:'installed',version:'2.10.0',previous:'2.6.0',target:'2.10.0',direction:'upgrade',observed_at:'2026-09-27T14:00:00Z'}],
  archives:[{id:7,installed_version:'2.6.0',target_version:'2.10.0',source:{kind:'github',url:'https://github.com/example/project',software:'Project'},notes:[{version:'2.10.0',url:'https://github.com/example/project/releases/tag/v2.10.0',body:'Notes',missing:false}],incomplete:false,captured_at:'2026-09-26T12:00:00Z',current:false}],
  analyses:[{notes:[],id:3,revision:4,installed_version:'2.6.0',target_version:'2.10.0',source:{kind:'github',url:'https://github.com/example/project',software:'Project'},result:{overview:[],details:[]},model:'test',created_at:'2026-09-26T13:00:00Z',current:false}]
 });
 assert.deepEqual(softwareJournal(base).map((entry) => entry.kind), ['observation','analysis','notes']);
 assert.deepEqual(softwareJournal(base).map((entry) => entry.at), ['2026-09-27T14:00:00Z','2026-09-26T13:00:00Z','2026-09-26T12:00:00Z']);
});
test('a current collection appears once in the journal even before archival', () => {
 const current = service({checked_at:'2026-09-27T12:00:00Z',collection_revision:4,collection:{installed_version:'2.6.0',target_version:'2.10.0',notes:[],incomplete:false}});
 assert.equal(softwareJournal(current).filter((entry) => entry.kind === 'notes').length,1);
 current.archives=[{id:8,installed_version:'2.6.0',target_version:'2.10.0',source:current.source,notes:[],incomplete:false,captured_at:'2026-09-27T12:00:00Z',current:true}];
 assert.equal(softwareJournal(current).filter((entry) => entry.kind === 'notes').length,1);
});
test('updates are ordered by group, security mention, level and name', () => {
 const services = [
  service({id:'current',name:'Alpha',group:'current',situation:'current',level:undefined}),
  service({id:'patch-b',name:'Beta',level:'patch'}),
  service({id:'patch-a',name:'alpha',level:'patch'}),
  service({id:'prerelease',name:'Gamma',group:'review',situation:'prerelease',level:'major'}),
  service({id:'unverified',name:'Zeta',group:'review',known:false}),
  service({id:'major',name:'Delta',level:'major'}),
  service({id:'security',name:'Omega',level:'patch',security_mentioned:true}),
 ];
 assert.deepEqual(services.sort(compareServices).map((s)=>s.id),['security','major','patch-a','patch-b','unverified','prerelease','current']);
});
test('only a rate-limited retry names the limited host', () => {
 assert.equal(rateLimitedHost(service({state:'retry',last_error:'rate_limited:api.github.com'})),'api.github.com');
 assert.equal(rateLimitedHost(service({state:'retry',last_error:'remote HTTP 403'})),undefined);
});
test('release links reject executable schemes and embedded credentials',()=>{
 for(const url of ['javascript:alert(1)','data:text/html,unsafe','https://user:secret@example.com'])assert.equal(safeReleaseURL(url),undefined);
 assert.equal(safeReleaseURL('https://github.com/example/project/releases'),'https://github.com/example/project/releases');
});
