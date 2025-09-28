 select  * from ai_analysis_results limit 4;
 select * from ai_analysis_sessions;
 select * from new_experiment_runs;
 
 -- count of completed AI analysis on a run
 select count(*) from ai_analysis_results air
 join new_experiment_runs ner on ner.run_id = air.session_id 
 where ner.run_name='sept_26';
 
 select * from new_experiment_runs;
 
 -- batches in a run regardless of whether AI is done
 select count(*) from new_batches nb
 join new_experiment_runs ner on ner.run_id = nb.run_id 
 where ner.run_name='sept_26';
 
 
 -- clusters in a run regardless of whether AI is done
 select count(*) from new_clusters nc
 join new_batches nb on nc.batch_id = nb.batch_number
 join new_experiment_runs ner on ner.run_id = nb.run_id 
 where ner.run_name='sept_26';
 
 
 -- batches in a run regardless of whether AI is done
 select 
 	(select count(*) from new_batches nb
 		join new_experiment_runs ner on ner.run_id = nb.run_id 
 		where ner.run_name='sept_26') as "ALL BATCHES", 
 	(select count(*) from ai_analysis_results air
 		join new_experiment_runs ner on ner.run_id = air.session_id 
 		where ner.run_name='sept_26') as "AI RESULTS",
 	(select count(*) from new_clusters nc
 		join new_batches nb on nc.batch_id = nb.batch_number
 		join new_experiment_runs ner on ner.run_id = nb.run_id 
 		where ner.run_name='sept_26') as "TOTAL CLUSTERS"
		
