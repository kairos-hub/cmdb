import request from './request'

export const getOverview = () => request.get('/stats/overview')
export const getAnalysis = () => request.get('/stats/analysis')
