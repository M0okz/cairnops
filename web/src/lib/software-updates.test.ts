import test from 'node:test';
import assert from 'node:assert/strict';
// @ts-ignore -- Node executes this test directly with its TypeScript loader.
import { compareServices, currentAnalysis, rateLimitedHost, safeReleaseURL, type SoftwareService } from './software-updates.ts';
function service(overrides: Partial<SoftwareService>): SoftwareService {
 return {id:'service',target_id:'target',resource_name:'Service',name:'owner/service',installed_version:'2.6.0',target_version:'2.10.0',observed_at:null,known:true,situation:'update',level:'minor',group:'apply',approved:false,skipped:false,security_mentioned:false,source:{kind:'github',url:'https://github.com/example/project',software:'Project'},source_origin:'manual',suggested_source:null,confirmed_at:null,revision:4,state:'pending',last_error:'',checked_at:null,next_check_at:null,collection:null,collection_revision:null,history:[],events:[],analyses:[],...overrides};
}
test('only the matching current comparison is presented as current', () => {
 const current = service({analyses:[{notes:[],id:1,revision:3,installed_version:'2.6.0',target_version:'2.9.1',source:{kind:'github',url:'https://github.com/example/project',software:'Project'},result:{overview:[],details:[]},model:'test',created_at:'2026-09-21T00:00:00Z',current:true}]});
 assert.equal(currentAnalysis(current),undefined);
 current.analyses[0].revision=4; current.analyses[0].target_version='2.10.0';assert.equal(currentAnalysis(current)?.id,1);
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
