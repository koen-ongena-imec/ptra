// PTRA: Patient Trajectory Analysis Library
// Copyright (c) 2022 imec vzw.

// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version, and Additional Terms
// (see below).

// This program is distributed in the hope that it will be useful, but
// WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
// Affero General Public License for more details.

// You should have received a copy of the GNU Affero General Public
// License and Additional Terms along with this program. If not, see
// <https://github.com/ExaScience/ptra/blob/master/LICENSE.txt>.

package cluster

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"ptra/trajectory"
	"ptra/utils"
	"strconv"
)

// jaccardTrajectory computes the Jaccard similarity coefficient for two given trajectories.
func jaccardTrajectory(t1, t2 *trajectory.Trajectory) float64 {
	// intersect t1 and t2
	n := 0
	for _, d1 := range t1.Diagnoses {
		if utils.MemberInt(d1, t2.Diagnoses) {
			n++
		}
	}
	nt1 := len(t1.Diagnoses)
	nt2 := len(t2.Diagnoses)
	return float64(n) / (float64(nt1) + float64(nt2) - float64(n))
}

// SzymkiewiczSimpsonTrajectory computes the Szymkiewicz-Simpson similarity coefficient for two given trajectories.
func SzymkiewiczSimpsonTrajectory(t1, t2 *trajectory.Trajectory) float64 {
	n := 0
	for _, d1 := range t1.Diagnoses {
		if utils.MemberInt(d1, t2.Diagnoses) {
			n++
		}
	}
	nt1 := len(t1.Diagnoses)
	nt2 := len(t2.Diagnoses)
	return float64(n) / float64(utils.MinInt(nt1, nt2))
}

// SorensenDiceTrajectory computes the SorensenDice similarity coefficient for two given trajectories.
func SorensenDiceTrajectory(t1, t2 *trajectory.Trajectory) float64 {
	n := 0
	for _, d1 := range t1.Diagnoses {
		if utils.MemberInt(d1, t2.Diagnoses) {
			n++
		}
	}
	nt1 := len(t1.Diagnoses)
	nt2 := len(t2.Diagnoses)
	return float64(2*n) / (float64(nt1 + nt2))
}

// convertTrajectoriesToAbcFormat compute the jaccard between each trajectory and writes out the result to file.
// Streaming algorithm to avoid pressure on memory.
func convertTrajectoriesToAbcFormat(exp *trajectory.Experiment, name string) {
	//create output file
	file, err := os.Create(name)
	if err != nil {
		log.Panic(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Panic(err)
		}
	}()
	// compute the jacard index for the trajectories
	for i, t1 := range exp.Trajectories {
		t1.ID = i
		for j := i + 1; j < len(exp.Trajectories); j++ {
			t2 := exp.Trajectories[j]
			t2.ID = j
			coeff := jaccardTrajectory(t1, t2)
			fmt.Fprintf(file, "%d\t%d\t%f\n", i, j, coeff)
		}
	}
}

// ClusterTrajectoriesDirectly performs clustering of the trajectories that have been calculated for a given experiment.
// It does a pairwise comparison of all trajectories by calculating the jaccard similarity coefficients. Subsequently,
// MCL clustering is used to group the trajectories by jaccard similarity into clusters.
func ClusterTrajectoriesDirectly(exp *trajectory.Experiment, granularities []int, path, pathToMcl string) {
	fmt.Println("Clustering trajectories directly with MCL")
	// convert trajectories to abc format for the mcl tool
	dirName := fmt.Sprintf("%s-clusters-directly/", exp.Name)
	workingDir := filepath.Join(path, dirName) + string(filepath.Separator)
	fmt.Println("Working path becomes: ", workingDir)
	derr := os.MkdirAll(workingDir, 0777)
	if derr != nil {
		panic(derr)
	}
	// change working dir cause mcl program dumps files into working dir
	os.Chdir(workingDir)
	abcFileName := fmt.Sprintf("%s%s.abc", workingDir, exp.Name)
	convertTrajectoriesToAbcFormat(exp, abcFileName)
	tabFileName := fmt.Sprintf("%s%s.tab", workingDir, exp.Name)
	mciFileName := fmt.Sprintf("%s%s.mci", workingDir, exp.Name)
	mcxloadCmd := fmt.Sprintf("%smcxload", pathToMcl)
	cmd := exec.Command(mcxloadCmd, "-abc", abcFileName, "--stream-mirror", "-write-tab", tabFileName, "-o", mciFileName)
	var out bytes.Buffer
	var serr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &serr
	err := cmd.Run()
	if err != nil {
		panic(err)
	}
	fmt.Println("Output: ", out.String(), serr.String())
	// run the clusterings with different granularities
	for _, gran := range granularities {
		mcl_cmd := fmt.Sprintf("%smcl", pathToMcl)
		cmd := exec.Command(mcl_cmd, mciFileName, "-I", fmt.Sprintf("%f", float64(gran)/10.0))
		var out2 bytes.Buffer
		var serr2 bytes.Buffer
		cmd.Stdout = &out2
		cmd.Stderr = &serr2
		fmt.Println("Output: ", out2.String(), serr2.String())
		err := cmd.Run()
		if err != nil {
			panic(err)
		}
	}
	// convert the clusterings to readable format
	clusterFileName := fmt.Sprintf("out.%s.mci", exp.Name)
	outFileName := fmt.Sprintf("dump.%s.mci", exp.Name)
	mcxdumpCmd := fmt.Sprintf("%smcxdump", pathToMcl)
	for _, gran := range granularities {
		cmd := exec.Command(mcxdumpCmd, "-icl", fmt.Sprintf("%s.I%d", clusterFileName, gran), "-tabr", tabFileName, "-o", fmt.Sprintf("%s.I%d", outFileName, gran))
		fmt.Println(mcxdumpCmd, "-icl", fmt.Sprintf("%s.I%d", clusterFileName, gran), "-tabr", tabFileName, "-o", fmt.Sprintf("%s.I%d", outFileName, gran))
		var out1 bytes.Buffer
		var serr1 bytes.Buffer
		cmd.Stdout = &out1
		cmd.Stderr = &serr1
		err := cmd.Run()
		fmt.Println("Output: ", out1.String(), serr1.String())
		if err != nil {
			panic(err)
		}
	}
	// convert the clusterings generated by mcl tool to gml format
	for _, gran := range granularities {
		dumpFileName := fmt.Sprintf("%s.I%d", outFileName, gran)
		convertToDirectTrajectoryClusterGraphs(exp, dumpFileName, fmt.Sprintf("%s.trajectories.gml", dumpFileName))
		convertToDirectTrajectoryClusterGraphsRR(exp, dumpFileName, fmt.Sprintf("%s.trajectories.RR.gml", dumpFileName))
		trajectory.PrintClusteredTrajectoriesToFile(exp, fmt.Sprintf("%s.clustered.trajectories.tab", dumpFileName))
		trajectory.PrintClustersToCSVFiles(exp, fmt.Sprintf("%s.clustered.patients.csv", dumpFileName),
			fmt.Sprintf("%s.clustered.clusters.csv", dumpFileName))
	}
}

// collectTrajectoriesFromClusterData looks up trajectories associated with a given list of trajectory ids and assigns
// each of these to a specific cluster id. It returns the list of trajectory objects.
func collectTrajectoriesFromClusterData(exp *trajectory.Experiment, ids []int, clusterID int) []*trajectory.Trajectory {
	ts := []*trajectory.Trajectory{}
	for _, id := range ids {
		// assign cluster label to trajectory
		exp.Trajectories[id].Cluster = clusterID
		ts = append(ts, exp.Trajectories[id])
	}
	return ts
}

// convertToDirectTrajectoryClusterGraphs produces a GML graph file for the clustered trajectories in an experiment. For
// this, it parses the cluster output from MCL, which is a file that lists for each cluster id a list of trajectory ids
// that are assigned to it. Then it looks up the concrete trajectory objects for each trajectory id. Finally, each
// cluster is written to the output file by writing all of the cluster's trajectories as part of a subgraph for that
// cluster.
func convertToDirectTrajectoryClusterGraphs(exp *trajectory.Experiment, input, output string) {
	file, err := os.Open(input)
	if err != nil {
		panic(err)
	}
	ofile, oerr := os.Create(output)
	if oerr != nil {
		panic(oerr)
	}
	defer func() {
		if err := file.Close(); err != nil {
			panic(err)
		}
		if oerr := ofile.Close(); oerr != nil {
			panic(oerr)
		}
	}()
	// trajectories to assign to clusters
	nofClusters := 0

	// parse file
	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		// collect codes in the cluster
		var codes []int
		for _, rcode := range record {
			code, err := strconv.Atoi(rcode)
			if err != nil {
				panic(err)
			}
			codes = append(codes, code)
		}
		// print the trajectories in the cluster
		collected := collectTrajectoriesFromClusterData(exp, codes, nofClusters)
		nofClusters++
		// print this cluster
		// print header
		fmt.Fprintf(ofile, "graph [ \n directed 1 \n multigraph 1\n")
		nodePrinted := map[int]bool{}
		// print nodes
		for _, t := range collected {
			for _, node := range t.Diagnoses {
				if _, ok := nodePrinted[node]; !ok {
					fmt.Fprintf(ofile, fmt.Sprintf("node [ id %d\n label \"%s\"\n ]\n", node, exp.NameMap[node]))
					nodePrinted[node] = true
				}
			}
		}
		// print edges
		edgePrinted := make([][][]int, exp.NofDiagnosisCodes)
		for i, _ := range edgePrinted {
			edgePrinted[i] = make([][]int, exp.NofDiagnosisCodes)
		}
		for _, t := range collected {
			d1 := t.Diagnoses[0]
			for i := 1; i < len(t.Diagnoses); i++ {
				d2 := t.Diagnoses[i]
				n := t.PatientNumbers[i-1]
				printed := edgePrinted[d1][d2]
				if !utils.MemberInt(n, printed) {
					fmt.Fprintf(ofile, fmt.Sprintf("edge [\nsource %d\ntarget %d\nlabel %d\n]\n", d1, d2, n))
					if printed == nil {
						edgePrinted[d1][d2] = []int{n}
					} else {
						edgePrinted[d1][d2] = append(edgePrinted[d1][d2], n)
					}
				}
				d1 = d2
			}
		}
		fmt.Fprintf(ofile, "]\n")
	}
	fmt.Println("For ", output)
	fmt.Println("Collected ", nofClusters, " clusters")
}

// percentMalesFemales computes for a given list of patients the percentage of males and females wrt to the total number
// of males and females in the experiment.
func percentMalesFemales(exp *trajectory.Experiment, ps []*trajectory.Patient) (float64, float64) {
	m := 0
	f := 0
	for _, p := range ps {
		if p.Sex == trajectory.Male {
			m++
		} else {
			f++
		}
	}
	return (100.0 / float64(exp.MCtr)) * float64(m), (100.0 / float64(exp.FCtr)) * float64((f))
}

// getDiagnosisDate returns the concrete diagnosis date for a given pair of diagnosis ids.
func getDiagnosisDate(p *trajectory.Patient, d1, d2 int) trajectory.DiagnosisDate {
	d1idx := -1
	d2idx := -1
	for i, d := range p.Diagnoses {
		if d.DID == d1 {
			d1idx = i
			continue
		}
		if d.DID == d2 && d1idx != -1 {
			d2idx = i
		}
	}
	return p.Diagnoses[d2idx].Date
}

// percentEOI computes percent of patients that have their event of interest at the time of the transition of disease
// d1 -> d2
func percentEOI(exp *trajectory.Experiment, ps []*trajectory.Patient, d1, d2 int) float64 {
	eoictr := 0
	for _, p := range ps {
		d := getDiagnosisDate(p, d1, d2)
		if p.EOIDate != nil && trajectory.DiagnosisDateSmallerThan(*p.EOIDate, d) {
			eoictr++
		}
	}
	return (100.0 / float64(len(ps))) * float64(eoictr)
}

func transitionInformation(exp *trajectory.Experiment, t *trajectory.Trajectory, i, d1, d2 int) (string, string, string) {
	rr := strconv.FormatFloat(exp.DxDRR[d1][d2], 'f', 2, 64)
	m, f := percentMalesFemales(exp, t.Patients[i])
	mfratio := strconv.FormatFloat(m/f, 'f', 2, 64)
	eoi := strconv.FormatFloat(percentEOI(exp, t.Patients[i], d1, d2), 'f', 0, 64)
	return rr, mfratio, eoi
}

// convertToDirectTrajectoryClusterGraphsRR converts MCL cluster output - a file with for each cluster id a list of
// trajectory ids - to a GML output file that plots the trajectories as graphs. Each cluster is plotted as a separate
// subgraph, with diagnosis codes used as nodes and trajectory transitions used as edges. The edges are annotated with
// the relatitive risk score (RR) associated with the diagnosis pair that the edge represents.
func convertToDirectTrajectoryClusterGraphsRR(exp *trajectory.Experiment, input, output string) {
	file, err := os.Open(input)
	if err != nil {
		panic(err)
	}
	ofile, oerr := os.Create(output)
	if oerr != nil {
		panic(oerr)
	}
	defer func() {
		if err := file.Close(); err != nil {
			panic(err)
		}
		if oerr := ofile.Close(); oerr != nil {
			panic(oerr)
		}
	}()
	// trajectories to assign to clusters
	nofClusters := 0

	// parse file
	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		// collect codes in the cluster
		var codes []int
		for _, rcode := range record {
			code, err := strconv.Atoi(rcode)
			if err != nil {
				panic(err)
			}
			codes = append(codes, code)
		}
		// print the trajectories in the cluster
		collected := collectTrajectoriesFromClusterData(exp, codes, nofClusters)
		nofClusters++
		// print this cluster
		// print header
		fmt.Fprintf(ofile,
			fmt.Sprintf("graph [ \n comment \"cluster %d\" \n directed 1 \n label \"cluster %d\" \n "+
				"multigraph 1\n", nofClusters-1, nofClusters-1))
		nodePrinted := map[int]bool{}
		// print nodes
		for _, t := range collected {
			for _, node := range t.Diagnoses {
				if _, ok := nodePrinted[node]; !ok {
					fmt.Fprintf(ofile, fmt.Sprintf("node [ id %d\n label \"%s\"\n ]\n", node, exp.NameMap[node]))
					nodePrinted[node] = true
				}
			}
		}
		// print edges
		edgePrinted := make([][]bool, exp.NofDiagnosisCodes)
		for i, _ := range edgePrinted {
			edgePrinted[i] = make([]bool, exp.NofDiagnosisCodes)
		}
		for _, t := range collected {
			d1 := t.Diagnoses[0]
			tctr := 0
			for i := 1; i < len(t.Diagnoses); i++ {
				d2 := t.Diagnoses[i]
				if !edgePrinted[d1][d2] {
					edgePrinted[d1][d2] = true
					RR := strconv.FormatFloat(exp.DxDRR[d1][d2], 'f', 2, 64)
					fmt.Fprintf(ofile, fmt.Sprintf("edge [\nsource %d\ntarget %d\nlabel %s\n]\n", d1, d2, RR))
					//rr, mfratio, eoi := transitionInformation(exp, t, tctr, d1, d2)
					//fmt.Fprintf(ofile, fmt.Sprintf("edge [\nsource %d\ntarget %d\nlabel \"RR:%s,M/F:%s,EOI:%s\"\n]\n", d1, d2, rr, mfratio, eoi))
				}
				d1 = d2
				tctr++
			}
		}
		fmt.Fprintf(ofile, "]\n")
	}
	fmt.Println("For ", output)
	fmt.Println("Collected ", nofClusters, " clusters")
}
